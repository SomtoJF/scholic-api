package endpoints

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"scholic/internal/database"
	feedlog "scholic/internal/feed"
	"scholic/internal/limits"
	"scholic/internal/models"
	"scholic/internal/services/ai"
	"scholic/internal/services/media"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	documentPageLimit = 20
)

type DocumentResponse struct {
	ID             uint      `json:"id"`
	CreatedAt      time.Time `json:"createdAt"`
	UserID         uint      `json:"userId"`
	User           gin.H     `json:"user"`
	Title          string    `json:"title"`
	Description    string    `json:"description"`
	Authors        []string  `json:"authors"`
	Tags           []string  `json:"tags"`
	PublishedYear  *int      `json:"publishedYear,omitempty"`
	Publisher      string    `json:"publisher"`
	PageCount      int       `json:"pageCount"`
	FileURL        string    `json:"fileUrl"`
	FileName       string    `json:"fileName"`
	FileSize       int64     `json:"fileSize"`
	MimeType       string    `json:"mimeType"`
	MetadataSource string    `json:"metadataSource"`
}

type CreateDocumentRequest struct {
	FileURL        string   `json:"fileUrl" binding:"required"`
	FileKey        string   `json:"fileKey" binding:"required"`
	FileName       string   `json:"fileName" binding:"required"`
	FileSize       int64    `json:"fileSize"`
	MimeType       string   `json:"mimeType"`
	Title          string   `json:"title" binding:"required"`
	Description    string   `json:"description"`
	Authors        []string `json:"authors"`
	Tags           []string `json:"tags"`
	PublishedYear  *int     `json:"publishedYear"`
	Publisher      string   `json:"publisher"`
	PageCount      int      `json:"pageCount"`
	MetadataSource string   `json:"metadataSource"`
}

type CancelDocumentPreviewRequest struct {
	FileKey string `json:"fileKey" binding:"required"`
}

// UploadDocumentPreview handles POST /api/upload/document-preview
func UploadDocumentPreview(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	if !media.IsR2Available() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "File storage is not configured"})
		return
	}

	file, header, err := c.Request.FormFile("document")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No document provided"})
		return
	}
	defer file.Close()

	contentType, err := sniffPDF(file, header)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if header.Size > limits.MaxDocumentUploadBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File too large. Maximum size is 15MB"})
		return
	}
	if used := sumDocumentBytesForUser(userID.(uint)); used+header.Size > limits.MaxDocumentTotalBytes {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":     "Storage limit reached. Delete some documents to upload more.",
			"usedBytes": used,
			"capBytes":  limits.MaxDocumentTotalBytes,
		})
		return
	}

	result, err := media.UploadFile(c.Request.Context(), file, header, "documents")
	if err != nil {
		log.Printf("[Documents] Failed to upload preview for user %v: %v", userID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload document"})
		return
	}

	meta, err := ai.ExtractDocumentMetadata(c.Request.Context(), result.URL, header.Filename)
	if err != nil {
		log.Printf("[Documents] Metadata extraction failed for %s: %v", header.Filename, err)
		meta = ai.DocumentMetadata{
			Title:          strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename)),
			Authors:        []string{},
			Tags:           []string{},
			MetadataSource: "fallback",
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"draft": gin.H{
			"fileUrl":  result.URL,
			"fileKey":  result.Key,
			"fileName": result.FileName,
			"fileSize": result.FileSize,
			"mimeType": contentType,
		},
		"metadata": meta,
	})
}

// CancelDocumentPreview handles POST /api/upload/document-preview/cancel
func CancelDocumentPreview(c *gin.Context) {
	_, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	if !media.IsR2Available() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "File storage is not configured"})
		return
	}

	var req CancelDocumentPreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !strings.HasPrefix(req.FileKey, "documents/") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid document key"})
		return
	}
	if err := media.DeleteFile(c.Request.Context(), req.FileKey); err != nil {
		log.Printf("[Documents] Failed to cancel staged preview %s: %v", req.FileKey, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete staged document"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// CreateDocument handles POST /api/documents
func CreateDocument(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	userID := userIDVal.(uint)

	var req CreateDocumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !strings.HasPrefix(req.FileKey, "documents/") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid document key"})
		return
	}
	if req.FileSize <= 0 || req.FileSize > limits.MaxDocumentUploadBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file size"})
		return
	}
	if used := sumDocumentBytesForUser(userID); used+req.FileSize > limits.MaxDocumentTotalBytes {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":     "Storage limit reached. Delete some documents to upload more.",
			"usedBytes": used,
			"capBytes":  limits.MaxDocumentTotalBytes,
		})
		return
	}
	if req.MimeType == "" {
		req.MimeType = "application/pdf"
	}
	if req.MimeType != "application/pdf" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Only PDFs are supported"})
		return
	}

	meta := ai.DocumentMetadata{
		Title:          strings.TrimSpace(req.Title),
		Authors:        req.Authors,
		PublishedYear:  req.PublishedYear,
		Description:    strings.TrimSpace(req.Description),
		Tags:           req.Tags,
		Publisher:      strings.TrimSpace(req.Publisher),
		PageCount:      req.PageCount,
		MetadataSource: strings.TrimSpace(req.MetadataSource),
	}
	if meta.Title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Title is required"})
		return
	}
	if meta.MetadataSource == "" {
		meta.MetadataSource = "manual"
	}

	authorsJSON, err := json.Marshal(req.Authors)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid authors"})
		return
	}
	tagsJSON, err := json.Marshal(req.Tags)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tags"})
		return
	}

	searchText := ai.BuildSearchText(meta)
	embedding, embErr := ai.GenerateEmbedding(c.Request.Context(), searchText)
	if embErr != nil {
		log.Printf("[Documents] Embedding generation failed: %v", embErr)
	}
	// jsonb rejects an empty string; keep the column valid when embedding
	// generation is unavailable and lexical search must be used.
	embeddingJSON := "null"
	if len(embedding) > 0 {
		if encoded, err := json.Marshal(embedding); err == nil {
			embeddingJSON = string(encoded)
		}
	}

	document := models.Document{
		UserID:         userID,
		Title:          meta.Title,
		Description:    meta.Description,
		AuthorsJSON:    string(authorsJSON),
		TagsJSON:       string(tagsJSON),
		PublishedYear:  meta.PublishedYear,
		Publisher:      meta.Publisher,
		PageCount:      meta.PageCount,
		FileURL:        req.FileURL,
		FileKey:        req.FileKey,
		FileName:       req.FileName,
		FileSize:       req.FileSize,
		MimeType:       req.MimeType,
		MetadataSource: meta.MetadataSource,
		SearchText:     searchText,
		EmbeddingJSON:  embeddingJSON,
	}

	if err := database.DB.Create(&document).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create document"})
		return
	}
	if err := database.DB.Preload("User").First(&document, document.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Document created but failed to reload"})
		return
	}
	feedlog.CreateEventAsync(feedlog.EventInput{
		ActorID:    document.UserID,
		Type:       models.FeedEventDocumentUploaded,
		TargetType: models.FeedTargetDocument,
		TargetID:   document.ID,
		IsPrivate:  false,
	})

	c.JSON(http.StatusCreated, formatDocument(document))
}

// GetUserDocuments handles GET /api/users/:username/documents
func GetUserDocuments(c *gin.Context) {
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

	limit, offset := parseListPagination(c)
	var total int64
	database.DB.Model(&models.Document{}).Where("user_id = ? AND deleted_at IS NULL", user.ID).Count(&total)

	var documents []models.Document
	if err := database.DB.Preload("User").
		Where("user_id = ? AND deleted_at IS NULL", user.ID).
		Order("created_at DESC").
		Limit(limit).Offset(offset).
		Find(&documents).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load documents"})
		return
	}

	items := make([]DocumentResponse, 0, len(documents))
	for _, document := range documents {
		items = append(items, formatDocument(document))
	}

	c.JSON(http.StatusOK, gin.H{
		"documents": items,
		"total":     total,
		"limit":     limit,
		"offset":    offset,
		"hasMore":   int64(offset+len(items)) < total,
	})
}

// GetDocument handles GET /api/documents/:id
func GetDocument(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid document ID"})
		return
	}

	var document models.Document
	if err := database.DB.Preload("User").First(&document, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Document not found"})
		return
	}

	c.JSON(http.StatusOK, formatDocument(document))
}

// GetDocumentByURL handles GET /api/documents/by-url?url=...
// Returns the document matching a file URL (omitting heavy fields like
// search_text and embedding via formatDocument). Used by SitePage to detect
// when a /page?url= request is actually an uploaded PDF so it can show real
// metadata and link back to the document page.
func GetDocumentByURL(c *gin.Context) {
	fileURL := strings.TrimSpace(c.Query("url"))
	if fileURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url query parameter is required"})
		return
	}

	var document models.Document
	if err := database.DB.Preload("User").Where("file_url = ?", fileURL).First(&document).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Document not found"})
		return
	}

	c.JSON(http.StatusOK, formatDocument(document))
}

func formatDocument(document models.Document) DocumentResponse {
	return DocumentResponse{
		ID:             document.ID,
		CreatedAt:      document.CreatedAt,
		UserID:         document.UserID,
		User:           gin.H{"id": document.User.ID, "username": document.User.Username, "firstName": document.User.FirstName, "lastName": document.User.LastName, "profilePicture": document.User.ProfilePicture},
		Title:          document.Title,
		Description:    document.Description,
		Authors:        decodeStringArray(document.AuthorsJSON),
		Tags:           decodeStringArray(document.TagsJSON),
		PublishedYear:  document.PublishedYear,
		Publisher:      document.Publisher,
		PageCount:      document.PageCount,
		FileURL:        document.FileURL,
		FileName:       document.FileName,
		FileSize:       document.FileSize,
		MimeType:       document.MimeType,
		MetadataSource: document.MetadataSource,
	}
}

// sumDocumentBytesForUser returns the total file_size (bytes) of a user's
// non-deleted documents. Used to gate uploads against limits.MaxDocumentTotalBytes.
func sumDocumentBytesForUser(userID uint) int64 {
	var result struct {
		Total int64
	}
	database.DB.Model(&models.Document{}).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Select("COALESCE(SUM(file_size), 0) AS total").
		Scan(&result)
	return result.Total
}

func decodeStringArray(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return []string{}
	}
	sort.Strings(values)
	return values
}

func encodeStringArray(values []string) string {
	clean := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		clean = append(clean, value)
	}
	sort.Strings(clean)
	if len(clean) == 0 {
		return "[]"
	}
	encoded, err := json.Marshal(clean)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

func parseListPagination(c *gin.Context) (int, int) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", strconv.Itoa(documentPageLimit)))
	if err != nil || limit <= 0 || limit > 50 {
		limit = documentPageLimit
	}
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 {
		offset = 0
	}
	return limit, offset
}

func sniffPDF(file multipart.File, header *multipart.FileHeader) (string, error) {
	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return "", err
	}
	if _, err := file.Seek(0, 0); err != nil {
		return "", err
	}
	contentType := http.DetectContentType(buffer[:n])
	if contentType != "application/pdf" && header.Header.Get("Content-Type") != "application/pdf" {
		return "", errors.New("Invalid file type. Only PDF files are allowed")
	}
	return "application/pdf", nil
}

type scoredDocument struct {
	document models.Document
	score    float64
}

func searchDocuments(ctx context.Context, query, username string, limit, offset int) ([]models.Document, int64) {
	query = strings.TrimSpace(query)
	if query == "" && username == "" {
		return []models.Document{}, 0
	}

	db := database.DB.Model(&models.Document{}).Where("documents.deleted_at IS NULL")
	if username != "" {
		var user models.User
		if err := database.DB.Select("id").Where("username = ?", username).First(&user).Error; err != nil {
			return []models.Document{}, 0
		}
		db = db.Where("documents.user_id = ?", user.ID)
	}

	var total int64
	db.Count(&total)

	if total == 0 {
		return []models.Document{}, 0
	}

	if query == "" {
		var documents []models.Document
		db.Preload("User").Order("documents.created_at DESC").Limit(limit).Offset(offset).Find(&documents)
		return documents, total
	}

	queryEmbedding, err := ai.GenerateEmbedding(ctx, query)
	if err != nil {
		log.Printf("[Documents] Query embedding failed: %v", err)
	}

	var candidates []models.Document
	db.Preload("User").Order("documents.created_at DESC").Limit(200).Find(&candidates)
	if len(queryEmbedding) == 0 {
		return lexicalDocumentSearch(db, query, limit, offset)
	}

	scored := make([]scoredDocument, 0, len(candidates))
	for _, document := range candidates {
		score := keywordScore(document.SearchText, query)
		if embedding := decodeEmbedding(document.EmbeddingJSON); len(embedding) > 0 {
			score += cosineSimilarity(queryEmbedding, embedding) * 4
		}
		if score <= 0 {
			continue
		}
		scored = append(scored, scoredDocument{document: document, score: score})
	}
	if len(scored) == 0 {
		return lexicalDocumentSearch(db, query, limit, offset)
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].document.CreatedAt.After(scored[j].document.CreatedAt)
		}
		return scored[i].score > scored[j].score
	})

	if offset >= len(scored) {
		return []models.Document{}, int64(len(scored))
	}
	end := offset + limit
	if end > len(scored) {
		end = len(scored)
	}
	out := make([]models.Document, 0, end-offset)
	for _, item := range scored[offset:end] {
		out = append(out, item.document)
	}
	return out, int64(len(scored))
}

func lexicalDocumentSearch(db *gorm.DB, query string, limit, offset int) ([]models.Document, int64) {
	like := "%" + query + "%"
	filtered := db.Where("documents.search_text ILIKE ?", like)
	var total int64
	filtered.Count(&total)
	var documents []models.Document
	filtered.Preload("User").Order("documents.created_at DESC").Limit(limit).Offset(offset).Find(&documents)
	return documents, total
}

func decodeEmbedding(raw string) []float64 {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var values []float64
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}
	return values
}

func keywordScore(haystack, query string) float64 {
	haystack = strings.ToLower(haystack)
	query = strings.ToLower(strings.TrimSpace(query))
	if haystack == "" || query == "" {
		return 0
	}
	score := 0.0
	if strings.Contains(haystack, query) {
		score += 2.5
	}
	for _, token := range strings.Fields(query) {
		if strings.Contains(haystack, token) {
			score += 0.75
		}
	}
	return score
}

func cosineSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, magA, magB float64
	for i := range a {
		dot += a[i] * b[i]
		magA += a[i] * a[i]
		magB += b[i] * b[i]
	}
	if magA == 0 || magB == 0 {
		return 0
	}
	return dot / (sqrt(magA) * sqrt(magB))
}

func sqrt(value float64) float64 {
	guess := value
	if guess == 0 {
		return 0
	}
	for i := 0; i < 8; i++ {
		guess = 0.5 * (guess + value/guess)
	}
	return guess
}
