package main

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// ConfigFile 配置文件结构
type ConfigFile struct {
	UDPListen          string `yaml:"udp_listen"`
	KCPQServer         string `yaml:"kcpq_server"`
	Subject            string `yaml:"subject"`
	AES256KeyHex       string `yaml:"aes256_key_hex"`
	StatsInterval      int    `yaml:"stats_interval"`
	BufferSize         int    `yaml:"buffer_size"`
	AutoReconnect      bool   `yaml:"auto_reconnect"`
	ReconnectInterval  int    `yaml:"reconnect_interval"`
}

// LoadConfig 加载配置文件
func LoadConfig(path string) (*ConfigFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg ConfigFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// 设置默认值
	if cfg.StatsInterval == 0 {
		cfg.StatsInterval = 10
	}
	if cfg.BufferSize == 0 {
		cfg.BufferSize = 10 * 1024 * 1024 // 10MB
	}
	if cfg.ReconnectInterval == 0 {
		cfg.ReconnectInterval = 5
	}

	return &cfg, nil
}

// ToRelayConfig 转换为 RelayConfig
func (c *ConfigFile) ToRelayConfig() *RelayConfig {
	return &RelayConfig{
		UDPListenAddr:     c.UDPListen,
		KCPServerAddr:     c.KCPQServer,
		Subject:           c.Subject,
		AES256KeyHex:      c.AES256KeyHex,
		ReconnectDelay:    3 * time.Second, // 废弃
		StatsInterval:     time.Duration(c.StatsInterval) * time.Second,
		BufferSize:        c.BufferSize,
		AutoReconnect:     c.AutoReconnect,
		ReconnectInterval: time.Duration(c.ReconnectInterval) * time.Second,
	}
}
