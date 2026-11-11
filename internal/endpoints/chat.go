package endpoints

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"scholic/internal/agents"
	"scholic/internal/database"
	"scholic/internal/models"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ChatMessage struct {
	ID            uint      `json:"id"`
	CreatedAt     time.Time `json:"createdAt"`
	IsUserMessage bool      `json:"isUserMessage"`
	MessageText   string    `json:"messageText"`
}

type GetChatResponse struct {
	ChatID   uint          `json:"chatId"`
	Messages []ChatMessage `json:"messages"`
}

type SendChatRequest struct {
	MessageText string `json:"messageText"`
}

type ChatSocketResponse struct {
	Type    string       `json:"type"`
	Message *ChatMessage `json:"message,omitempty"`
	Output  *string      `json:"output,omitempty"`
	Error   string       `json:"error,omitempty"`
}

type ChatHandler struct{ Agent agents.Agent }

func NewChatHandler(agent agents.Agent) *ChatHandler { return &ChatHandler{Agent: agent} }

// GetChat handles GET /api/chats/by-url?url=<url>.
// It creates the authenticated user's chat for the URL when missing.
func (h *ChatHandler) GetChat(c *gin.Context) {
	userID := c.MustGet("userID").(uint)
	pageURL := canonicalAnnotationURL(strings.TrimSpace(c.Query("url")))
	if pageURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url query parameter is required"})
		return
	}

	ctx := c.Request.Context()
	chat, err := getOrCreateChat(ctx, userID, pageURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load chat"})
		return
	}

	messages, err := loadChatMessages(ctx, chat.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load chat messages"})
		return
	}

	c.JSON(http.StatusOK, GetChatResponse{
		ChatID:   chat.ID,
		Messages: messages,
	})
}

func getOrCreateChat(ctx context.Context, userID uint, pageURL string) (*models.Chat, error) {
	chat := &models.Chat{}
	db := database.DB.WithContext(ctx)
	if err := db.Where("user_id = ? AND url = ?", userID, pageURL).First(chat).Error; err == nil {
		return chat, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	chat = &models.Chat{UserID: userID, URL: pageURL}
	if err := db.Create(chat).Error; err != nil {
		// A concurrent request may have created the unique row first.
		if lookupErr := db.Where("user_id = ? AND url = ?", userID, pageURL).First(chat).Error; lookupErr == nil {
			return chat, nil
		}
		return nil, err
	}
	return chat, nil
}

func loadChatMessages(ctx context.Context, chatID uint) ([]ChatMessage, error) {
	messages := []ChatMessage{}
	err := database.DB.WithContext(ctx).
		Model(&models.Message{}).
		Select("id, created_at, is_user_message, message_text").
		Where("chat_id = ?", chatID).
		Order("created_at ASC, id ASC").
		Find(&messages).Error
	return messages, err
}

// SendToChat handles GET /api/chats/:id/messages.
// The WebSocket accepts {"messageText":"..."} and persists user messages.
func (h *ChatHandler) SendToChat(c *gin.Context) {
	userID := c.MustGet("userID").(uint)
	chatID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat id"})
		return
	}

	chat, err := getChatForUser(c.Request.Context(), userID, uint(chatID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "chat not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load chat"})
		return
	}

	conn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
		OriginPatterns: []string{
			"http://localhost:5173",
			"http://localhost:5174",
			"http://localhost:5175",
			"http://localhost:5176",
			"https://scholic.com",
			"https://www.scholic.com",
			"chrome-extension://*",
		},
	})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	conn.SetReadLimit(16 * 1024)

	ctx := c.Request.Context()

	for {
		msgType, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		if msgType != websocket.MessageText {
			_ = writeChatError(ctx, conn, "only text messages are supported")
			continue
		}

		var request SendChatRequest
		if err := json.Unmarshal(data, &request); err != nil {
			_ = writeChatError(ctx, conn, "invalid message payload")
			continue
		}
		request.MessageText = strings.TrimSpace(request.MessageText)
		if request.MessageText == "" {
			_ = writeChatError(ctx, conn, "messageText is required")
			continue
		}

		message, err := persistUserMessage(ctx, chat.ID, request.MessageText)
		if err != nil {
			_ = writeChatError(ctx, conn, "failed to save message")
			continue
		}

		response := ChatSocketResponse{
			Type: "user_message",
			Message: &ChatMessage{
				ID:            message.ID,
				CreatedAt:     message.CreatedAt,
				IsUserMessage: true,
				MessageText:   message.MessageText,
			},
		}
		if err := wsjson.Write(ctx, conn, response); err != nil {
			return
		}

		if h.Agent == nil {
			_ = writeChatError(ctx, conn, "chat agent is not configured")
			continue
		}
		transport := &webSocketToolTransport{conn: conn}
		runOutput, err := h.Agent.Run(ctx, agents.RunInput{
			UserID: userID, ChatID: chat.ID, PageURL: chat.URL,
			Message: request.MessageText, Transport: transport,
		})
		if err != nil {
			_ = writeChatError(ctx, conn, err.Error())
			continue
		}
		if runOutput.Output == nil {
			continue
		}
		assistant, err := persistChatMessage(ctx, chat.ID, false, *runOutput.Output)
		if err != nil {
			_ = writeChatError(ctx, conn, "failed to save assistant message")
			continue
		}
		if err := wsjson.Write(ctx, conn, ChatSocketResponse{Type: "assistant_message", Message: chatMessageFromModel(assistant)}); err != nil {
			return
		}
	}
}

func getChatForUser(ctx context.Context, userID, chatID uint) (*models.Chat, error) {
	chat := &models.Chat{}
	err := database.DB.WithContext(ctx).Where("id = ? AND user_id = ?", chatID, userID).First(chat).Error
	return chat, err
}

func persistUserMessage(ctx context.Context, chatID uint, text string) (*models.Message, error) {
	return persistChatMessage(ctx, chatID, true, text)
}

func persistChatMessage(ctx context.Context, chatID uint, isUserMessage bool, text string) (*models.Message, error) {
	message := &models.Message{ChatID: chatID, IsUserMessage: isUserMessage, MessageText: text}
	if err := database.DB.WithContext(ctx).Create(message).Error; err != nil {
		return nil, err
	}
	return message, nil
}

func chatMessageFromModel(message *models.Message) *ChatMessage {
	return &ChatMessage{ID: message.ID, CreatedAt: message.CreatedAt, IsUserMessage: message.IsUserMessage, MessageText: message.MessageText}
}

func writeChatError(ctx context.Context, conn *websocket.Conn, message string) error {
	return wsjson.Write(ctx, conn, ChatSocketResponse{Type: "error", Error: message})
}
