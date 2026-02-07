package model

import (
	"path/filepath"
	"testing"
)

func TestInitDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	// 验证表已创建
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()

	// 检查 conversations 表
	var count int
	err = sqlDB.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='conversations'").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Error("conversations table not created")
	}

	// 检查 messages 表
	err = sqlDB.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='messages'").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Error("messages table not created")
	}
}

func TestCRUD(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	// 创建 Conversation
	conv := Conversation{ID: "test-conv-1", Title: "Test Conversation"}
	if err := db.Create(&conv).Error; err != nil {
		t.Fatalf("create conversation failed: %v", err)
	}

	// 创建 Message
	msg := Message{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Hello",
		Status:         "pending",
	}
	if err := db.Create(&msg).Error; err != nil {
		t.Fatalf("create message failed: %v", err)
	}

	// 查询验证
	var loaded Message
	if err := db.First(&loaded, msg.ID).Error; err != nil {
		t.Fatalf("query message failed: %v", err)
	}
	if loaded.Content != "Hello" {
		t.Errorf("expected content 'Hello', got '%s'", loaded.Content)
	}
	if loaded.ConversationID != "test-conv-1" {
		t.Errorf("expected conversation_id 'test-conv-1', got '%s'", loaded.ConversationID)
	}
}
