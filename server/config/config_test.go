package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	content := `
server:
  port: 9090
database:
  path: "./test.db"
websocket:
  ping_interval: 15
  pong_timeout: 5
`
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Database.Path != "./test.db" {
		t.Errorf("expected db path ./test.db, got %s", cfg.Database.Path)
	}
	if cfg.WebSocket.PingInterval != 15 {
		t.Errorf("expected ping_interval 15, got %d", cfg.WebSocket.PingInterval)
	}
	if cfg.WebSocket.PongTimeout != 5 {
		t.Errorf("expected pong_timeout 5, got %d", cfg.WebSocket.PongTimeout)
	}
}

func TestLoadDefaults(t *testing.T) {
	content := `{}`
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Server.Port != 6543 {
		t.Errorf("expected default port 6543, got %d", cfg.Server.Port)
	}
	if cfg.Database.Path != "./data.db" {
		t.Errorf("expected default db path ./data.db, got %s", cfg.Database.Path)
	}
	if cfg.WebSocket.PingInterval != 30 {
		t.Errorf("expected default ping_interval 30, got %d", cfg.WebSocket.PingInterval)
	}
	if cfg.WebSocket.PongTimeout != 10 {
		t.Errorf("expected default pong_timeout 10, got %d", cfg.WebSocket.PongTimeout)
	}
}

func TestLoadFileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}
