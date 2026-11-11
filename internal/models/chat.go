package models

import (
	"time"

	"gorm.io/gorm"
)

type Chat struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// UserID links this chat to the user who created it
	UserID uint `gorm:"uniqueIndex:idx_chat_user_url;not null" json:"userId"`
	User   User `gorm:"foreignKey:UserID" json:"user"`

	// URL is the page where the chat was created (indexed for fast lookup)
	URL string `gorm:"uniqueIndex:idx_chat_user_url;not null;size:2000" json:"url"`

	Messages []Message `gorm:"foreignKey:ChatID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"messages,omitempty"`
}
