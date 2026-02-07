package model

import (
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type Conversation struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
}

type Message struct {
	ID             uint         `gorm:"primaryKey;autoIncrement" json:"id"`
	ConversationID string       `gorm:"index" json:"conversation_id"`
	Conversation   Conversation `gorm:"foreignKey:ConversationID" json:"-"`
	Role           string       `json:"role"`   // "user" or "model"
	Content        string       `gorm:"type:text" json:"content"`
	Status         string       `json:"status"` // "pending", "sent", "received", "error"
	CreatedAt      time.Time    `json:"created_at"`
}

func InitDB(dbPath string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(&Conversation{}, &Message{}); err != nil {
		return nil, err
	}

	return db, nil
}
