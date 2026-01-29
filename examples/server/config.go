package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type ConfigFile struct {
	ListenAddr   string `yaml:"listen_addr"`
	PprofAddr    string `yaml:"pprof_addr"`
	AES256KeyHex string `yaml:"aes256_key_hex"`
}

func LoadConfig(path string) (*ConfigFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg ConfigFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":4000"
	}
	if cfg.PprofAddr == "" {
		cfg.PprofAddr = "localhost:6060"
	}

	return &cfg, nil
}

