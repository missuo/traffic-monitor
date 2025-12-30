package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"traffic-monitor/api"
	"traffic-monitor/config"
	"traffic-monitor/proxy"
	"traffic-monitor/stats"
)

type Proxy interface {
	Start() error
	Stop()
}

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	statsManager := stats.NewStatsManager()

	persistence := stats.NewPersistence(cfg.DataFile, statsManager)
	if err := persistence.Load(); err != nil {
		log.Printf("Warning: Failed to load persisted stats: %v", err)
	}

	var proxies []Proxy

	for _, p := range cfg.Proxies {
		proxyStats := statsManager.Register(p.Name, p.Protocol, p.ListenPort, p.TargetPort)

		switch p.Protocol {
		case "tcp":
			tcpProxy := proxy.NewTCPProxy(p.Name, p.ListenPort, p.TargetHost, p.TargetPort, proxyStats)
			if err := tcpProxy.Start(); err != nil {
				log.Fatalf("Failed to start TCP proxy %s: %v", p.Name, err)
			}
			proxies = append(proxies, tcpProxy)

		case "udp":
			udpProxy, err := proxy.NewUDPProxy(p.Name, p.ListenPort, p.TargetHost, p.TargetPort, proxyStats)
			if err != nil {
				log.Fatalf("Failed to create UDP proxy %s: %v", p.Name, err)
			}
			if err := udpProxy.Start(); err != nil {
				log.Fatalf("Failed to start UDP proxy %s: %v", p.Name, err)
			}
			proxies = append(proxies, udpProxy)

		case "both":
			// TCP and UDP share the same stats
			tcpProxy := proxy.NewTCPProxy(p.Name, p.ListenPort, p.TargetHost, p.TargetPort, proxyStats)
			if err := tcpProxy.Start(); err != nil {
				log.Fatalf("Failed to start TCP proxy %s: %v", p.Name, err)
			}
			proxies = append(proxies, tcpProxy)

			udpProxy, err := proxy.NewUDPProxy(p.Name, p.ListenPort, p.TargetHost, p.TargetPort, proxyStats)
			if err != nil {
				log.Fatalf("Failed to create UDP proxy %s: %v", p.Name, err)
			}
			if err := udpProxy.Start(); err != nil {
				log.Fatalf("Failed to start UDP proxy %s: %v", p.Name, err)
			}
			proxies = append(proxies, udpProxy)

		default:
			log.Fatalf("Unknown protocol %s for proxy %s", p.Protocol, p.Name)
		}
	}

	persistence.Start(30 * time.Second)

	apiServer := api.NewServer(cfg.API.Port, cfg.API.Token, statsManager)
	if err := apiServer.Start(); err != nil {
		log.Fatalf("Failed to start API server: %v", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down...")

	apiServer.Stop()

	for _, p := range proxies {
		p.Stop()
	}

	persistence.Stop()

	log.Println("Shutdown complete")
}
