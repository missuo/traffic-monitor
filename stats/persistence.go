package stats

import (
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"
)

type Persistence struct {
	filePath string
	manager  *StatsManager
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

func NewPersistence(filePath string, manager *StatsManager) *Persistence {
	return &Persistence{
		filePath: filePath,
		manager:  manager,
		stopCh:   make(chan struct{}),
	}
}

func (p *Persistence) Load() error {
	data, err := os.ReadFile(p.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var stats map[string]*ProxyStats
	if err := json.Unmarshal(data, &stats); err != nil {
		return err
	}

	p.manager.SetStats(stats)
	log.Printf("Loaded stats from %s", p.filePath)
	return nil
}

func (p *Persistence) Save() error {
	stats := p.manager.GetStatsMap()
	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := p.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}

	if err := os.Rename(tmpFile, p.filePath); err != nil {
		return err
	}

	return nil
}

func (p *Persistence) Start(interval time.Duration) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := p.Save(); err != nil {
					log.Printf("Failed to save stats: %v", err)
				}
			case <-p.stopCh:
				return
			}
		}
	}()
}

func (p *Persistence) Stop() {
	close(p.stopCh)
	p.wg.Wait()

	if err := p.Save(); err != nil {
		log.Printf("Failed to save stats on shutdown: %v", err)
	} else {
		log.Printf("Stats saved to %s", p.filePath)
	}
}
