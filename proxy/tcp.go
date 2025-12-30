package proxy

import (
	"fmt"
	"io"
	"log"
	"net"
	"sync"

	"github.com/missuo/traffic-monitor/stats"
)

var bufferPool = sync.Pool{
	New: func() interface{} {
		buf := make([]byte, 32*1024) // 32KB buffer
		return &buf
	},
}

type TCPProxy struct {
	name       string
	listenAddr string
	targetAddr string
	stats      *stats.ProxyStats
	listener   net.Listener
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

func NewTCPProxy(name string, listenPort int, targetHost string, targetPort int, s *stats.ProxyStats) *TCPProxy {
	return &TCPProxy{
		name:       name,
		listenAddr: fmt.Sprintf(":%d", listenPort),
		targetAddr: fmt.Sprintf("%s:%d", targetHost, targetPort),
		stats:      s,
		stopCh:     make(chan struct{}),
	}
}

func (p *TCPProxy) Start() error {
	listener, err := net.Listen("tcp", p.listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", p.listenAddr, err)
	}
	p.listener = listener
	log.Printf("[TCP] %s: listening on %s -> %s", p.name, p.listenAddr, p.targetAddr)

	p.wg.Add(1)
	go p.acceptLoop()

	return nil
}

func (p *TCPProxy) Stop() {
	close(p.stopCh)
	if p.listener != nil {
		p.listener.Close()
	}
	p.wg.Wait()
}

func (p *TCPProxy) acceptLoop() {
	defer p.wg.Done()

	for {
		select {
		case <-p.stopCh:
			return
		default:
		}

		conn, err := p.listener.Accept()
		if err != nil {
			select {
			case <-p.stopCh:
				return
			default:
				log.Printf("[TCP] %s: accept error: %v", p.name, err)
				continue
			}
		}

		go p.handleConn(conn)
	}
}

func (p *TCPProxy) handleConn(src net.Conn) {
	defer src.Close()

	if p.stats.IsLimitExceeded() {
		log.Printf("[TCP] %s: connection rejected, traffic limit exceeded", p.name)
		return
	}

	dst, err := net.Dial("tcp", p.targetAddr)
	if err != nil {
		log.Printf("[TCP] %s: failed to connect to target %s: %v", p.name, p.targetAddr, err)
		return
	}
	defer dst.Close()

	var wg sync.WaitGroup
	wg.Add(2)

	// Client -> Target (Upload)
	go func() {
		defer wg.Done()
		p.copy(dst, src, true)
		dst.(*net.TCPConn).CloseWrite()
	}()

	// Target -> Client (Download)
	go func() {
		defer wg.Done()
		p.copy(src, dst, false)
		src.(*net.TCPConn).CloseWrite()
	}()

	wg.Wait()
}

func (p *TCPProxy) copy(dst, src net.Conn, isUpload bool) {
	bufPtr := bufferPool.Get().(*[]byte)
	defer bufferPool.Put(bufPtr)
	buf := *bufPtr

	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			written, writeErr := dst.Write(buf[:n])
			if written > 0 {
				if isUpload {
					p.stats.AddUpload(int64(written))
				} else {
					p.stats.AddDownload(int64(written))
				}
			}
			if writeErr != nil {
				return
			}
		}
		if readErr != nil {
			if readErr != io.EOF {
				// Log non-EOF errors if needed
			}
			return
		}
	}
}
