package main

import (
	"log"
	"os"
	"strings"
	"time"

	"scholic/internal/agents"
	"scholic/internal/blocklist"
	"scholic/internal/database"
	"scholic/internal/discover"
	"scholic/internal/endpoints"
	"scholic/internal/initializers"
	"scholic/internal/middleware"
	"scholic/internal/services/ai"
	"scholic/internal/services/media"
	"scholic/internal/utils/jwt"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// buildAllowedOrigins merges hardcoded local dev origins with any production
// origins supplied via the ALLOWED_ORIGINS environment variable.
func buildAllowedOrigins() map[string]struct{} {
	origins := map[string]struct{}{
		"http://localhost:5173": {},
		"http://localhost:5174": {},
		"http://localhost:5175": {},
		"http://localhost:5176": {},
	}
	if extra := os.Getenv("ALLOWED_ORIGINS"); extra != "" {
		for _, o := range strings.Split(extra, ",") {
			o = strings.TrimSpace(o)
			if o != "" {
				origins[o] = struct{}{}
			}
		}
	}
	return origins
}

func main() {
	initializers.InitializeEnv()
	database.InitializeDatabase()
	database.InitializeRedis()
	initializers.InitGothic()
	jwt.InitJWT()
	blocklist.LoadAndCache()
	ai.InitGemini()
	chatAgent := agents.NewChatAgent(nil, []agents.Tool{
		agents.SearchChatHistoryTool(),
		agents.GetChatHistoryRangeTool(),
		agents.SearchPageContentTool(),
		agents.SearchAnnotationsTool(),
		agents.CommentOnAnnotationTool(),
	}, agents.LoadRecentChatMessages)
	chatHandler := endpoints.NewChatHandler(chatAgent)

	// Initialize R2 (optional — server works without it, just can't upload files)
	if err := media.InitializeR2(); err != nil {
		log.Printf("[Warning] R2 not initialized: %v", err)
		log.Println("[Warning] File upload features will be unavailable")
	}

	// Discover precompute job — runs an immediate warm-up tick then every 3h.
	go discover.Run(3 * time.Hour)

	r := gin.Default()

	// CORS Middleware — allow both the web client and browser extension.
	// In production set ALLOWED_ORIGINS to a comma-separated list of web origins
	// e.g. "https://scholic.com,https://www.scholic.com"
	allowedOrigins := buildAllowedOrigins()

	config := cors.DefaultConfig()
	config.AllowOriginFunc = func(origin string) bool {
		if strings.HasPrefix(origin, "chrome-extension://") {
			return true
		}
		_, ok := allowedOrigins[origin]
		return ok
	}
	config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"}
	config.AllowHeaders = []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With"}
	config.AllowCredentials = true
	config.MaxAge = 12 * time.Hour

	r.Use(cors.New(config))

	// Per-IP token-bucket throttle. Sits in front of every route so even
	// unauthenticated traffic gets bounded.
	r.Use(middleware.IPRateLimit())

	// ── Auth Routes (Outside /api to match Turva's Google Callback) ──
	auth := r.Group("/auth")
	{
		auth.GET("/:provider", endpoints.AuthGoogle)
		auth.GET("/:provider/callback", endpoints.AuthGoogleCallback)
		auth.POST("/logout", endpoints.AuthLogout)

		// Protected auth routes
		protected := auth.Group("/")
		protected.Use(middleware.AuthMiddleware())
		{
			protected.GET("/me", endpoints.AuthMe)
			protected.POST("/onboard", endpoints.AuthOnboard)
		}
	}

	api := r.Group("/api")
	{
		// ── Auth Mirror (for client convenience) ──
		apiAuth := api.Group("/auth")
		{
			apiAuth.GET("/me", middleware.AuthMiddleware(), endpoints.AuthMe)
			apiAuth.GET("/token", middleware.AuthMiddleware(), endpoints.AuthToken)
			apiAuth.POST("/onboard", middleware.AuthMiddleware(), endpoints.AuthOnboard)
			apiAuth.POST("/logout", endpoints.AuthLogout)

			// Profile management
			apiAuth.PUT("/profile", middleware.AuthMiddleware(), endpoints.UpdateProfile)
			apiAuth.GET("/check-username", middleware.OptionalAuthMiddleware(), endpoints.CheckUsername)
		}

		// ── User Search ──
		api.GET("/users/search", middleware.OptionalAuthMiddleware(), endpoints.SearchUsers)

		// ── Public User Profiles ──
		api.GET("/users/:username", middleware.OptionalAuthMiddleware(), endpoints.GetUserProfile)
		api.GET("/users/:username/followers", middleware.OptionalAuthMiddleware(), endpoints.GetFollowers)
		api.GET("/users/:username/following", middleware.OptionalAuthMiddleware(), endpoints.GetFollowing)
		api.GET("/users/:username/recently-read", middleware.OptionalAuthMiddleware(), endpoints.GetRecentlyRead)
		api.GET("/users/:username/friends-reading", middleware.OptionalAuthMiddleware(), endpoints.GetFriendsReading)
		api.GET("/users/:username/bookmarks", middleware.OptionalAuthMiddleware(), endpoints.GetUserBookmarks)
		api.GET("/users/:username/collections", middleware.OptionalAuthMiddleware(), endpoints.GetUserCollections)
		api.POST("/users/:username/follow", middleware.AuthMiddleware(), endpoints.ToggleFollow)

		// ── Search ──
		api.GET("/search", middleware.OptionalAuthMiddleware(), endpoints.Search)

		// ── Feed ──
		api.GET("/feed", middleware.OptionalAuthMiddleware(), endpoints.GetFeed)

		// ── Discover ──
		api.GET("/discover", middleware.OptionalAuthMiddleware(), endpoints.GetDiscover)
		api.POST("/admin/discover/refresh", endpoints.RefreshDiscover)

		// ── Blocklist (extension fetches this; public, no auth) ──
		api.GET("/blocklist", endpoints.GetBlocklist)

		// ── Bookmarks ──
		bookmarksProtected := api.Group("/bookmarks")
		bookmarksProtected.Use(middleware.AuthMiddleware())
		{
			bookmarksProtected.GET("/by-url", endpoints.GetBookmarkByURL)
			bookmarksProtected.POST("", endpoints.CreateBookmark)
			bookmarksProtected.PATCH("/by-url", endpoints.UpdateBookmarkByURL)
			bookmarksProtected.DELETE("/by-url", endpoints.DeleteBookmarkByURL)
		}

		// ── Chats ──
		chatsProtected := api.Group("/chats")
		chatsProtected.Use(middleware.AuthMiddleware())
		{
			chatsProtected.GET("/by-url", chatHandler.GetChat)
			chatsProtected.GET("/:id/messages", chatHandler.SendToChat)
		}

		// ── Collections ──
		api.GET("/collections/:id", middleware.OptionalAuthMiddleware(), endpoints.GetCollection)
		collectionsProtected := api.Group("/collections")
		collectionsProtected.Use(middleware.AuthMiddleware())
		{
			collectionsProtected.GET("", endpoints.GetMyCollections)
			collectionsProtected.POST("", endpoints.CreateCollection)
			collectionsProtected.PATCH("/:id", endpoints.UpdateCollection)
			collectionsProtected.DELETE("/:id", endpoints.DeleteCollection)
			collectionsProtected.POST("/:id/cover", endpoints.UploadCollectionCover)
			collectionsProtected.DELETE("/:id/cover", endpoints.DeleteCollectionCover)
			collectionsProtected.POST("/:id/entries", endpoints.AddCollectionEntry)
			collectionsProtected.DELETE("/:id/entries/:entryId", endpoints.DeleteCollectionEntry)
		}

		// ── Notifications ──
		notificationsProtected := api.Group("/notifications")
		notificationsProtected.Use(middleware.AuthMiddleware())
		{
			notificationsProtected.GET("", endpoints.GetNotifications)
			notificationsProtected.GET("/unread-count", endpoints.GetUnreadNotificationCount)
			notificationsProtected.POST("/read-all", endpoints.MarkAllNotificationsRead)
			notificationsProtected.POST("/:id/read", endpoints.MarkNotificationRead)
		}

		// ── Site Page ──
		api.GET("/site", middleware.OptionalAuthMiddleware(), endpoints.GetSitePage)

		// ── Documents ──
		api.GET("/documents/by-url", middleware.OptionalAuthMiddleware(), endpoints.GetDocumentByURL)
		api.GET("/documents/:id", middleware.OptionalAuthMiddleware(), endpoints.GetDocument)
		api.POST("/documents", middleware.AuthMiddleware(), endpoints.CreateDocument)
		api.GET("/users/:username/documents", middleware.OptionalAuthMiddleware(), endpoints.GetUserDocuments)

		// ── Annotation Routes ──
		// GET uses optional auth (shows public + user's private annotations)
		api.GET("/annotations", middleware.OptionalAuthMiddleware(), endpoints.GetAnnotations)
		api.GET("/annotations/:id", middleware.OptionalAuthMiddleware(), endpoints.GetAnnotation)

		// ── Private page highlights ──
		highlightsProtected := api.Group("/highlights")
		highlightsProtected.Use(middleware.AuthMiddleware())
		{
			highlightsProtected.GET("", endpoints.GetHighlights)
			highlightsProtected.POST("", endpoints.CreateHighlight)
			highlightsProtected.PATCH("/:id", endpoints.UpdateHighlight)
			highlightsProtected.DELETE("/:id", endpoints.DeleteHighlight)
		}

		// POST, PUT, and DELETE require authentication
		annotationsProtected := api.Group("/annotations")
		annotationsProtected.Use(middleware.AuthMiddleware())
		{
			// Reads + deletes don't go through the write throttle.
			annotationsProtected.GET("/:id/comments", endpoints.GetComments)
			annotationsProtected.DELETE("/:id", endpoints.DeleteAnnotation)
			annotationsProtected.DELETE("/:id/comments/:commentId", endpoints.DeleteComment)
			annotationsProtected.DELETE("/:id/reactions", endpoints.ClearReaction)

			// Per-user write throttle protects feeds and notification fan-out
			// from spam without affecting reads.
			writes := annotationsProtected.Group("")
			writes.Use(middleware.UserWriteRateLimit())
			{
				writes.POST("", endpoints.CreateAnnotation)
				writes.PUT("/:id", endpoints.UpdateAnnotation)
				writes.POST("/:id/comments", endpoints.CreateComment)
				writes.PUT("/:id/comments/:commentId", endpoints.UpdateComment)
				writes.POST("/:id/reactions", endpoints.ToggleReaction)
			}
		}

		// ── Upload Routes ──
		uploadProtected := api.Group("/upload")
		uploadProtected.Use(middleware.AuthMiddleware())
		{
			uploadProtected.POST("/avatar", endpoints.UploadAvatar)
			uploadProtected.POST("/document-preview", endpoints.UploadDocumentPreview)
			uploadProtected.POST("/document-preview/cancel", endpoints.CancelDocumentPreview)
		}
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatal(err)
	}
}
