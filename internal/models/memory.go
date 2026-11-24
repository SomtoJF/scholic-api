package models

import (
	"time"

	"gorm.io/gorm"
)

type Memory struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// UserID links this chat to the user who created it
	UserID uint   `gorm:"index;not null" json:"userId"`
	User   User   `gorm:"foreignKey:UserID" json:"user"`
	ChatID *uint  `gorm:"index" json:"chatId,omitempty"`
	Chat   *Chat  `gorm:"foreignKey:ChatID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"chat,omitempty"`
	Text   string `json:"text"`
}
