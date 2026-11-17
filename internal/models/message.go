package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// StringSlice stores JSON arrays without allowing nil to become SQL NULL.
// page_references is NOT NULL, so an empty slice must be written as `[]`.
type StringSlice []string

func (s StringSlice) Value() (driver.Value, error) {
	if s == nil {
		s = StringSlice{}
	}
	return json.Marshal([]string(s))
}

func (s *StringSlice) Scan(value any) error {
	if value == nil {
		*s = StringSlice{}
		return nil
	}
	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("cannot scan %T into StringSlice", value)
	}
	if len(data) == 0 || string(data) == "null" {
		*s = StringSlice{}
		return nil
	}
	return json.Unmarshal(data, s)
}

type Message struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	ChatID         uint        `gorm:"index;not null" json:"chatId"`
	Chat           Chat        `gorm:"foreignKey:ChatID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
	IsUserMessage  bool        `json:"isUserMessage"`
	MessageText    string      `json:"messageText"`
	PageReferences StringSlice `gorm:"type:jsonb;not null;default:'[]'" json:"pageReferences"`
}
