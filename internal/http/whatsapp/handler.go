package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"ai-smart-storage/internal/ai"
	"ai-smart-storage/internal/database"
	service "ai-smart-storage/internal/whatsapp"

	"github.com/gofiber/fiber/v2"
)

type Handler struct {
	ai          *ai.Client
	store       *database.Store
	wa          *service.Service
	signupURL   string
	cacheTTL    time.Duration
	cacheMu     sync.Mutex
	accessCache map[string]cachedAccess
}

type cachedAccess struct {
	access    database.WhatsAppAccess
	expiresAt time.Time
}

func New(aiClient *ai.Client, store *database.Store, wa *service.Service, signupURL string) *Handler {
	return &Handler{ai: aiClient, store: store, wa: wa, signupURL: signupURL, cacheTTL: 3 * time.Minute, accessCache: make(map[string]cachedAccess)}
}

func (h *Handler) Register(app fiber.Router) {
	app.Get("/webhooks/whatsapp", h.Verify)
	app.Post("/webhooks/whatsapp", h.Receive)
	app.Get("/v1/wa-conversations", h.ListConversations)
}

func (h *Handler) ListConversations(c *fiber.Ctx) error {
	userID, err := strconv.ParseUint(c.Query("user_id"), 10, 64)
	if err != nil || userID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "user_id is required")
	}
	items, err := h.store.WAConversations(c.Context(), userID)
	if err != nil {
		return fiber.ErrInternalServerError
	}
	return c.JSON(items)
}

func (h *Handler) Verify(c *fiber.Ctx) error {
	challenge, err := h.wa.Verify(c.Query("hub.mode"), c.Query("hub.challenge"), c.Query("hub.verify_token"))
	if err != nil {
		return fiber.ErrForbidden
	}
	return c.SendString(challenge)
}

func (h *Handler) Receive(c *fiber.Ctx) error {
	body := c.Body()
	if !h.wa.ValidSignature(body, c.Get("X-Hub-Signature-256")) {
		return fiber.ErrUnauthorized
	}
	var incoming service.Incoming
	if err := json.Unmarshal(body, &incoming); err != nil {
		return fiber.ErrBadRequest
	}
	for _, entry := range incoming.Entry {
		for _, change := range entry.Changes {
			for _, message := range change.Value.Messages {
				if message.Type == "text" {
					go h.reply(message.ID, message.From, message.Text.Body)
				}
			}
		}
	}
	return c.SendStatus(fiber.StatusOK)
}

func (h *Handler) reply(id, phone, text string) {
	ctx := context.Background()
	access, err := h.whatsAppAccess(ctx, phone)
	if errors.Is(err, database.ErrWhatsAppAccessNotFound) {
		h.sendNotice(ctx, phone, "Number not registered. Sign up: "+h.signupURL)
		return
	}
	if err != nil {
		log.Printf("check WhatsApp access: %v", err)
		return
	}
	if !access.WithinQuota() {
		h.sendNotice(ctx, phone, "Your plan has reached its limit or needs renewal. Please renew or upgrade to continue: "+h.signupURL)
		return
	}
	userID := access.UserID
	if err := h.store.LogWAConversation(ctx, database.WAConversation{UserID: userID, WAMessageID: id, Direction: "inbound", MessageType: "text", Category: "service", Content: text, Cost: "0"}); err != nil {
		log.Printf("log inbound WhatsApp message: %v", err)
	}
	if err := h.store.IncrementUsageQuota(ctx, userID, "0", 0, 0, 1); err != nil {
		log.Printf("increment inbound WhatsApp usage: %v", err)
	}
	if err := h.store.OpenWAWindow(ctx, userID, time.Now().UTC()); err != nil {
		log.Printf("open WhatsApp window: %v", err)
	}
	if err := h.store.SaveMessage(ctx, id, phone, "user", text); err != nil {
		log.Printf("save inbound message: %v", err)
		return
	}
	if err := h.store.IncrementUsageQuota(ctx, userID, "0", 0, 1, 0); err != nil {
		log.Printf("reserve AI query usage: %v", err)
		return
	}
	history, err := h.store.History(ctx, phone, 20)
	if err != nil {
		log.Printf("load history: %v", err)
		return
	}
	messages := make([]ai.Message, len(history))
	for i, item := range history {
		messages[i] = ai.Message{Role: item.Role, Content: item.Content}
	}
	var response strings.Builder
	if err := h.ai.Stream(ctx, messages, func(part string) error { response.WriteString(part); return nil }); err != nil {
		log.Printf("stream AI reply: %v", err)
		return
	}
	if access.InGracePeriod(time.Now().UTC()) {
		response.WriteString("\n\nYour subscription has expired. Please renew soon: ")
		response.WriteString(h.signupURL)
	}
	if err := h.store.SaveMessage(ctx, "", phone, "assistant", response.String()); err != nil {
		log.Printf("save AI message: %v", err)
		return
	}
	if err := h.wa.SendText(ctx, phone, response.String()); err != nil {
		log.Printf("send WhatsApp message: %v", err)
		return
	}
	open, err := h.store.WAWindowOpen(ctx, userID, time.Now().UTC())
	if err != nil {
		log.Printf("check WhatsApp window: %v", err)
	}
	category := "utility"
	if open {
		category = "service"
	}
	if err := h.store.LogWAConversation(ctx, database.WAConversation{UserID: userID, Direction: "outbound", MessageType: "text", Category: category, Content: response.String(), Cost: "0"}); err != nil {
		log.Printf("log outbound WhatsApp message: %v", err)
	}
	if err := h.store.IncrementUsageQuota(ctx, userID, "0", 0, 0, 1); err != nil {
		log.Printf("increment outbound WhatsApp usage: %v", err)
	}
}

func (h *Handler) whatsAppAccess(ctx context.Context, phone string) (database.WhatsAppAccess, error) {
	now := time.Now().UTC()
	h.cacheMu.Lock()
	item, ok := h.accessCache[phone]
	if ok && now.Before(item.expiresAt) {
		h.cacheMu.Unlock()
		return item.access, nil
	}
	h.cacheMu.Unlock()

	access, err := h.store.WhatsAppAccess(ctx, phone, now)
	if err != nil {
		return database.WhatsAppAccess{}, err
	}
	h.cacheMu.Lock()
	h.accessCache[phone] = cachedAccess{access: access, expiresAt: now.Add(h.cacheTTL)}
	h.cacheMu.Unlock()
	return access, nil
}

func (h *Handler) sendNotice(ctx context.Context, phone, text string) {
	if err := h.wa.SendText(ctx, phone, text); err != nil {
		log.Printf("send WhatsApp notice: %v", err)
	}
}
