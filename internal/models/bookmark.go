package models

import (
	"time"

	"gorm.io/gorm"
)

// Bookmark represents a user's saved page independent of annotations.
type Bookmark struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	UserID uint `gorm:"uniqueIndex:idx_user_bookmark_url;not null" json:"userId"`
	User   User `gorm:"foreignKey:UserID" json:"user"`

	URL     string `gorm:"uniqueIndex:idx_user_bookmark_url;not null;size:2000" json:"url"`
	Title   string `gorm:"size:500" json:"title"`
	Favicon string `gorm:"size:2048" json:"favicon"`
	Domain  string `gorm:"size:255" json:"domain"`

	// IsPrivate controls whether this bookmark is visible on the user's public profile.
	IsPrivate bool `gorm:"not null" json:"isPrivate"`
}
