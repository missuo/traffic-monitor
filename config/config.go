package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	API      APIConfig     `yaml:"api"`
	DataFile string        `yaml:"data_file"`
	Proxies  []ProxyConfig `yaml:"proxies"`
}

type APIConfig struct {
	Port  int    `yaml:"port"`
	Token string `yaml:"token"`
}

type ProxyConfig struct {
	Name         string `yaml:"name"`
	ListenPort   int    `yaml:"listen_port"`
	TargetHost   string `yaml:"target_host"`
	TargetPort   int    `yaml:"target_port"`
	Protocol     string `yaml:"protocol"`      // tcp, udp, or both
	Limit        string `yaml:"limit"`         // total limit, e.g., "100GB", "1TB", 0 = unlimited
	LimitMonthly string `yaml:"limit_monthly"` // monthly limit, e.g., "100GB", "1TB", 0 = unlimited
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Set defaults
	if cfg.API.Port == 0 {
		cfg.API.Port = 8080
	}
	if cfg.DataFile == "" {
		cfg.DataFile = "./traffic_data.json"
	}

	for i := range cfg.Proxies {
		if cfg.Proxies[i].Protocol == "" {
			cfg.Proxies[i].Protocol = "tcp"
		}
		if cfg.Proxies[i].TargetHost == "" {
			cfg.Proxies[i].TargetHost = "127.0.0.1"
		}
	}

	return &cfg, nil
}
