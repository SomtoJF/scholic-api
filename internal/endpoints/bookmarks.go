package endpoints

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"scholic/internal/database"
	feedlog "scholic/internal/feed"
	"scholic/internal/models"
	"scholic/internal/scraper"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type CreateBookmarkRequest struct {
	URL       string `json:"url" binding:"required"`
	Title     string `json:"title"`
	IsPrivate bool   `json:"isPrivate"`
}

type UpdateBookmarkRequest struct {
	IsPrivate *bool `json:"isPrivate" binding:"required"`
}

// GetBookmarkByURL handles GET /api/bookmarks/by-url?url=...
func GetBookmarkByURL(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	pageURL := c.Query("url")
	if pageURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url query parameter is required"})
		return
	}
	pageURL = canonicalAnnotationURL(pageURL)

	var bookmark models.Bookmark
	err := database.DB.Where("user_id = ? AND url = ?", userIDVal.(uint), pageURL).First(&bookmark).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusOK, gin.H{"bookmarked": false, "url": pageURL})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load bookmark"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"bookmarked": true, "bookmark": bookmark})
}

// CreateBookmark handles POST /api/bookmarks.
func CreateBookmark(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	userID := userIDVal.(uint)

	var req CreateBookmarkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	pageURL := canonicalAnnotationURL(req.URL)

	var existing models.Bookmark
	err := database.DB.Where("user_id = ? AND url = ?", userID, pageURL).First(&existing).Error
	if err == nil {
		if existing.IsPrivate != req.IsPrivate {
			existing.IsPrivate = req.IsPrivate
			if err := database.DB.Save(&existing).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update bookmark"})
				return
			}
			if err := feedlog.UpdateEventPrivacy(c.Request.Context(), models.FeedEventBookmarkCreated, models.FeedTargetBookmark, existing.ID, existing.IsPrivate); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update bookmark feed event"})
				return
			}
		}
		feedlog.CreateEventAsync(feedlog.EventInput{
			ActorID:    existing.UserID,
			Type:       models.FeedEventBookmarkCreated,
			TargetType: models.FeedTargetBookmark,
			TargetID:   existing.ID,
			IsPrivate:  existing.IsPrivate,
		})
		c.JSON(http.StatusOK, gin.H{"bookmarked": true, "bookmark": existing})
		return
	}
	if err != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load bookmark"})
		return
	}

	title, favicon, domain := bookmarkMetadata(pageURL, req.Title)
	bookmark := models.Bookmark{
		UserID:    userID,
		URL:       pageURL,
		Title:     title,
		Favicon:   favicon,
		Domain:    domain,
		IsPrivate: req.IsPrivate,
	}
	if err := database.DB.Select("UserID", "URL", "Title", "Favicon", "Domain", "IsPrivate").Create(&bookmark).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create bookmark"})
		return
	}
	feedlog.CreateEventAsync(feedlog.EventInput{
		ActorID:    bookmark.UserID,
		Type:       models.FeedEventBookmarkCreated,
		TargetType: models.FeedTargetBookmark,
		TargetID:   bookmark.ID,
		IsPrivate:  bookmark.IsPrivate,
	})

	c.JSON(http.StatusCreated, gin.H{"bookmarked": true, "bookmark": bookmark})
}

// UpdateBookmarkByURL handles PATCH /api/bookmarks/by-url?url=...
func UpdateBookmarkByURL(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	pageURL := c.Query("url")
	if pageURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url query parameter is required"})
		return
	}
	pageURL = canonicalAnnotationURL(pageURL)

	var req UpdateBookmarkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.IsPrivate == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "isPrivate is required"})
		return
	}

	var bookmark models.Bookmark
	if err := database.DB.Where("user_id = ? AND url = ?", userIDVal.(uint), pageURL).First(&bookmark).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Bookmark not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load bookmark"})
		return
	}

	bookmark.IsPrivate = *req.IsPrivate
	if err := database.DB.Save(&bookmark).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update bookmark"})
		return
	}
	if err := feedlog.UpdateEventPrivacy(c.Request.Context(), models.FeedEventBookmarkCreated, models.FeedTargetBookmark, bookmark.ID, bookmark.IsPrivate); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update bookmark feed event"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"bookmarked": true, "bookmark": bookmark})
}

// DeleteBookmarkByURL handles DELETE /api/bookmarks/by-url?url=...
func DeleteBookmarkByURL(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	pageURL := c.Query("url")
	if pageURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url query parameter is required"})
		return
	}
	pageURL = canonicalAnnotationURL(pageURL)

	var bookmark models.Bookmark
	err := database.DB.Where("user_id = ? AND url = ?", userIDVal.(uint), pageURL).First(&bookmark).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusOK, gin.H{"bookmarked": false, "url": pageURL})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load bookmark"})
		return
	}

	if err := feedlog.DeleteEvents(c.Request.Context(), models.FeedEventBookmarkCreated, models.FeedTargetBookmark, bookmark.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete bookmark feed event"})
		return
	}
	if err := database.DB.Unscoped().Delete(&bookmark).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete bookmark"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"bookmarked": false, "url": pageURL})
}

// GetUserBookmarks handles GET /api/users/:username/bookmarks.
func GetUserBookmarks(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username parameter is required"})
		return
	}

	var user models.User
	if err := database.DB.Where("username = ?", username).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	isOwner := false
	if currentUserID, exists := c.Get("userID"); exists && currentUserID.(uint) == user.ID {
		isOwner = true
	}

	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil || limit <= 0 || limit > 50 {
		limit = 20
	}
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 {
		offset = 0
	}

	query := database.DB.Model(&models.Bookmark{}).Where("user_id = ?", user.ID)
	if !isOwner {
		query = query.Where("is_private = ?", false)
	}

	var total int64
	query.Count(&total)

	var bookmarks []models.Bookmark
	if err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&bookmarks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load bookmarks"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"bookmarks": bookmarks,
		"total":     total,
		"limit":     limit,
		"offset":    offset,
		"hasMore":   int64(offset+len(bookmarks)) < total,
	})
}

func bookmarkMetadata(pageURL, fallbackTitle string) (title, favicon, domain string) {
	title = strings.TrimSpace(fallbackTitle)
	meta, err := scraper.ExtractMetadata(pageURL)
	if err == nil && meta != nil {
		if strings.TrimSpace(meta.Title) != "" {
			title = strings.TrimSpace(meta.Title)
		}
		favicon = meta.Favicon
		domain = meta.Domain
	}
	if domain == "" {
		if parsedURL, parseErr := url.Parse(pageURL); parseErr == nil {
			domain = strings.TrimPrefix(parsedURL.Host, "www.")
		}
	}
	if title == "" {
		title = domain
	}
	return title, favicon, domain
}
