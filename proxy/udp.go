package proxy

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"traffic-monitor/stats"
)

const (
	udpBufferSize   = 65535
	udpTimeout      = 60 * time.Second
	cleanupInterval = 30 * time.Second
)

type udpClient struct {
	targetConn *net.UDPConn
	clientAddr *net.UDPAddr
	lastActive time.Time
}

type UDPProxy struct {
	name       string
	listenAddr string
	targetAddr *net.UDPAddr
	stats      *stats.ProxyStats
	listener   *net.UDPConn
	clients    map[string]*udpClient
	clientsMu  sync.RWMutex
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

func NewUDPProxy(name string, listenPort int, targetHost string, targetPort int, s *stats.ProxyStats) (*UDPProxy, error) {
	targetAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", targetHost, targetPort))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve target address: %w", err)
	}

	return &UDPProxy{
		name:       name,
		listenAddr: fmt.Sprintf(":%d", listenPort),
		targetAddr: targetAddr,
		stats:      s,
		clients:    make(map[string]*udpClient),
		stopCh:     make(chan struct{}),
	}, nil
}

func (p *UDPProxy) Start() error {
	addr, err := net.ResolveUDPAddr("udp", p.listenAddr)
	if err != nil {
		return fmt.Errorf("failed to resolve listen address: %w", err)
	}

	listener, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", p.listenAddr, err)
	}
	p.listener = listener
	log.Printf("[UDP] %s: listening on %s -> %s", p.name, p.listenAddr, p.targetAddr.String())

	p.wg.Add(2)
	go p.readLoop()
	go p.cleanupLoop()

	return nil
}

func (p *UDPProxy) Stop() {
	close(p.stopCh)
	if p.listener != nil {
		p.listener.Close()
	}

	p.clientsMu.Lock()
	for _, client := range p.clients {
		client.targetConn.Close()
	}
	p.clientsMu.Unlock()

	p.wg.Wait()
}

func (p *UDPProxy) readLoop() {
	defer p.wg.Done()

	buf := make([]byte, udpBufferSize)
	for {
		select {
		case <-p.stopCh:
			return
		default:
		}

		p.listener.SetReadDeadline(time.Now().Add(time.Second))
		n, clientAddr, err := p.listener.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			select {
			case <-p.stopCh:
				return
			default:
				log.Printf("[UDP] %s: read error: %v", p.name, err)
				continue
			}
		}

		p.stats.AddUpload(int64(n))

		client := p.getOrCreateClient(clientAddr)
		if client == nil {
			continue
		}

		client.lastActive = time.Now()
		_, err = client.targetConn.Write(buf[:n])
		if err != nil {
			log.Printf("[UDP] %s: write to target error: %v", p.name, err)
		}
	}
}

func (p *UDPProxy) getOrCreateClient(clientAddr *net.UDPAddr) *udpClient {
	key := clientAddr.String()

	p.clientsMu.RLock()
	client, exists := p.clients[key]
	p.clientsMu.RUnlock()

	if exists {
		return client
	}

	p.clientsMu.Lock()
	defer p.clientsMu.Unlock()

	// Double check
	if client, exists = p.clients[key]; exists {
		return client
	}

	targetConn, err := net.DialUDP("udp", nil, p.targetAddr)
	if err != nil {
		log.Printf("[UDP] %s: failed to connect to target: %v", p.name, err)
		return nil
	}

	client = &udpClient{
		targetConn: targetConn,
		clientAddr: clientAddr,
		lastActive: time.Now(),
	}
	p.clients[key] = client

	// Start reading from target for this client
	go p.readFromTarget(client, key)

	return client
}

func (p *UDPProxy) readFromTarget(client *udpClient, key string) {
	buf := make([]byte, udpBufferSize)
	for {
		select {
		case <-p.stopCh:
			return
		default:
		}

		client.targetConn.SetReadDeadline(time.Now().Add(udpTimeout))
		n, err := client.targetConn.Read(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				p.clientsMu.RLock()
				lastActive := client.lastActive
				p.clientsMu.RUnlock()

				if time.Since(lastActive) > udpTimeout {
					p.removeClient(key)
					return
				}
				continue
			}
			select {
			case <-p.stopCh:
				return
			default:
				p.removeClient(key)
				return
			}
		}

		p.stats.AddDownload(int64(n))
		client.lastActive = time.Now()

		_, err = p.listener.WriteToUDP(buf[:n], client.clientAddr)
		if err != nil {
			log.Printf("[UDP] %s: write to client error: %v", p.name, err)
		}
	}
}

func (p *UDPProxy) removeClient(key string) {
	p.clientsMu.Lock()
	defer p.clientsMu.Unlock()

	if client, exists := p.clients[key]; exists {
		client.targetConn.Close()
		delete(p.clients, key)
	}
}

func (p *UDPProxy) cleanupLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.cleanupStaleClients()
		}
	}
}

func (p *UDPProxy) cleanupStaleClients() {
	p.clientsMu.Lock()
	defer p.clientsMu.Unlock()

	now := time.Now()
	for key, client := range p.clients {
		if now.Sub(client.lastActive) > udpTimeout {
			client.targetConn.Close()
			delete(p.clients, key)
		}
	}
}
