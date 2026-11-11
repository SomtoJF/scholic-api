package models

import (
	"time"

	"gorm.io/gorm"
)

type Message struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	ChatID        uint   `gorm:"index;not null" json:"chatId"`
	Chat          Chat   `gorm:"foreignKey:ChatID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
	IsUserMessage bool   `json:"isUserMessage"`
	MessageText   string `json:"messageText"`
}
