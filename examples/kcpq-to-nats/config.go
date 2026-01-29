package main

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// ConfigFile 配置文件结构
type ConfigFile struct {
	KCPQServer        string `yaml:"kcpq_server"`
	KCPQSubject       string `yaml:"kcpq_subject"`
	NATSServer        string `yaml:"nats_server"`
	NATSSubject       string `yaml:"nats_subject"`
	AES256KeyHex      string `yaml:"aes256_key_hex"`
	StatsInterval     int    `yaml:"stats_interval"`
	AutoReconnect     bool   `yaml:"auto_reconnect"`
	ReconnectInterval int    `yaml:"reconnect_interval"`
	UseChannelMode    bool   `yaml:"use_channel_mode"`
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
	if cfg.ReconnectInterval == 0 {
		cfg.ReconnectInterval = 5
	}

	return &cfg, nil
}

// ToForwarderConfig 转换为 ForwarderConfig
func (c *ConfigFile) ToForwarderConfig() *ForwarderConfig {
	return &ForwarderConfig{
		KCPQServer:        c.KCPQServer,
		KCPQSubject:       c.KCPQSubject,
		NATSServer:        c.NATSServer,
		NATSSubject:       c.NATSSubject,
		AES256KeyHex:      c.AES256KeyHex,
		ReconnectDelay:    3 * time.Second, // 废弃
		StatsInterval:     time.Duration(c.StatsInterval) * time.Second,
		AutoReconnect:     c.AutoReconnect,
		ReconnectInterval: time.Duration(c.ReconnectInterval) * time.Second,
		UseChannelMode:    c.UseChannelMode,
	}
}
