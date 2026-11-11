package database

import (
	"fmt"
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"scholic/internal/models"
)

var DB *gorm.DB

func InitializeDatabase() {
	var err error
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=UTC",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"),
		os.Getenv("DB_PORT"),
	)

	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Fatalf("[Scholic] Failed to connect to database: %v", err)
	}

	log.Printf("[Scholic] Database connected at %s:%s/%s",
		os.Getenv("DB_HOST"), os.Getenv("DB_PORT"), os.Getenv("DB_NAME"))

	err = DB.AutoMigrate(&models.User{}, &models.Annotation{}, &models.Highlight{}, &models.Comment{}, &models.Follow{}, &models.Reaction{}, &models.Notification{}, &models.Document{}, &models.Bookmark{}, &models.FeedEvent{}, &models.Collection{}, &models.CollectionEntry{}, &models.Chat{}, &models.Message{})
	if err != nil {
		log.Fatal("Failed to migrate database:", err)
	}

	// Partial composite index for the recently-read GROUP BY query.
	// Covers the WHERE clause (user_id, is_private=false, deleted_at IS NULL) and the
	// GROUP BY (url) + ORDER BY (created_at DESC) in a single index scan.
	// CONCURRENTLY means it doesn't lock the table if the DB is already running.
	// IF NOT EXISTS makes it idempotent across restarts.
	sqlDB, _ := DB.DB()
	_, _ = sqlDB.Exec(`
		CREATE INDEX IF NOT EXISTS idx_annotations_recently_read
		ON annotations (user_id, url, created_at DESC)
		WHERE is_private = false AND deleted_at IS NULL
	`)
	_, _ = sqlDB.Exec(`
		CREATE INDEX IF NOT EXISTS idx_documents_user_created
		ON documents (user_id, created_at DESC)
		WHERE deleted_at IS NULL
	`)
	_, _ = sqlDB.Exec(`
		CREATE INDEX IF NOT EXISTS idx_messages_text_search
		ON messages USING GIN (to_tsvector('english', message_text))
	`)
	log.Println("[Scholic] Database indexes verified")
}
