package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Database  DatabaseConfig  `yaml:"database"`
	WebSocket WebSocketConfig `yaml:"websocket"`
	APIKey    string          `yaml:"api_key"` // 可选，为空则不验证
}

type ServerConfig struct {
	Port int    `yaml:"port"`
	Mode string `yaml:"mode"` // debug/test/release
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type WebSocketConfig struct {
	PingInterval int `yaml:"ping_interval"`
	PongTimeout  int `yaml:"pong_timeout"`
}

// Default 返回默认配置
func Default() *Config {
	return &Config{
		Server:   ServerConfig{Port: 6543, Mode: "release"},
		Database: DatabaseConfig{Path: "./data.db"},
		WebSocket: WebSocketConfig{
			PingInterval: 30,
			PongTimeout:  10,
		},
	}
}

// Load 从文件加载配置，以默认值为基础覆盖
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
