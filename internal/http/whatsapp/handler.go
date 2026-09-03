package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"ai-smart-storage/internal/ai"
	"ai-smart-storage/internal/database"
	"ai-smart-storage/internal/http/middleware"
	phoneutil "ai-smart-storage/internal/phone"
	service "ai-smart-storage/internal/whatsapp"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/redis/go-redis/v9"
)

type Handler struct {
	ai        *ai.Client
	store     *database.Store
	wa        *service.Service
	signupURL string
	cacheTTL  time.Duration
	redis     *redis.Client
}

func New(aiClient *ai.Client, store *database.Store, wa *service.Service, signupURL string, redisClient *redis.Client) *Handler {
	return &Handler{ai: aiClient, store: store, wa: wa, signupURL: signupURL, redis: redisClient}
}

func (h *Handler) Register(app fiber.Router) {
	app.Get("/webhooks/whatsapp", h.Verify)
	app.Post("/webhooks/whatsapp", limiter.New(limiter.Config{Max: 120, Expiration: time.Minute}), h.Receive)
}

func (h *Handler) RegisterProtected(app fiber.Router) {
	app.Get("/v1/wa-conversations", h.ListConversations)
}

func (h *Handler) ListConversations(c *fiber.Ctx) error {
	userID, err := middleware.SelfID(c, c.Query("user_id"))
	if err != nil {
		return err
	}
	if h.store == nil {
		return fiber.ErrInternalServerError
	}
	limit := 20
	if l := c.QueryInt("limit", 20); l > 0 && l <= 100 {
		limit = l
	}
	offset := c.QueryInt("offset", 0)
	if offset < 0 {
		offset = 0
	}
	items, err := h.store.WAConversations(c.Context(), userID, limit, offset)
	if err != nil {
		return fiber.ErrInternalServerError
	}
	return c.JSON(items)
}

func (h *Handler) Verify(c *fiber.Ctx) error {
	if h.wa == nil {
		return fiber.ErrInternalServerError
	}
	challenge, err := h.wa.Verify(c.Query("hub.mode"), c.Query("hub.challenge"), c.Query("hub.verify_token"))
	if err != nil {
		return fiber.ErrForbidden
	}
	return c.SendString(challenge)
}

func (h *Handler) Receive(c *fiber.Ctx) error {
	if h.wa == nil {
		log.Printf("whatsapp: WA service not configured")
		return fiber.ErrInternalServerError
	}
	body := c.Body()
	if !h.wa.ValidSignature(body, c.Get("X-Hub-Signature-256")) {
		log.Printf("whatsapp: invalid signature")
		return fiber.ErrUnauthorized
	}
	var incoming service.Incoming
	if err := json.Unmarshal(body, &incoming); err != nil {
		log.Printf("whatsapp: unmarshal error: %v", err)
		return fiber.ErrBadRequest
	}
	textMessages := 0
	for _, entry := range incoming.Entry {
		for _, change := range entry.Changes {
			for _, message := range change.Value.Messages {
				textMessages++
				log.Printf("whatsapp: message type=%s from=%s id=%s", message.Type, message.From, message.ID)
				if message.Type == "text" {
					go h.reply(message.ID, message.From, message.Text.Body)
				}
			}
		}
	}
	if textMessages == 0 {
		log.Printf("whatsapp: received webhook with %d entries but no messages (likely status update)", len(incoming.Entry))
	}
	return c.SendStatus(fiber.StatusOK)
}

func (h *Handler) reply(id, phone, text string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("whatsapp reply panic: %v", r)
		}
	}()
	phone = phoneutil.Normalize(phone)
	log.Printf("whatsapp reply: from=%s text=%q", phone, text)
	if h.store == nil {
		log.Printf("whatsapp reply: store not configured")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
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
	if err := h.store.CheckQuota(ctx, userID, 0, 0, 1, 0); err != nil {
		if errors.Is(err, database.ErrQuotaExceeded) {
			h.sendNotice(ctx, phone, "Your AI query quota has been reached. Please upgrade your plan: "+h.signupURL)
			return
		}
		if errors.Is(err, database.ErrWhatsAppAccessNotFound) {
			h.sendNotice(ctx, phone, "Your subscription has expired. Please renew: "+h.signupURL)
			return
		}
		log.Printf("check AI quota: %v", err)
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
	if h.ai == nil {
		log.Printf("stream AI reply: AI client not configured")
		return
	}
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
	if h.wa == nil {
		log.Printf("send WhatsApp message: WA client not configured")
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
	if err := h.store.CheckQuota(ctx, userID, 0, 0, 0, 1); err != nil {
		if errors.Is(err, database.ErrQuotaExceeded) {
			log.Printf("outbound WA quota exceeded: %v", err)
		} else {
			log.Printf("check outbound WA quota: %v", err)
		}
	} else if err := h.store.IncrementUsageQuota(ctx, userID, "0", 0, 0, 1); err != nil {
		log.Printf("increment outbound WhatsApp usage: %v", err)
	}
}

func (h *Handler) whatsAppAccess(ctx context.Context, phone string) (database.WhatsAppAccess, error) {
	phone = phoneutil.Normalize(phone)
	if h.store == nil {
		return database.WhatsAppAccess{}, database.ErrWhatsAppAccessNotFound
	}
	now := time.Now().UTC()
	key := "whatsapp:access:" + phone
	if h.redis != nil {
		cached, err := h.redis.Get(ctx, key).Result()
		if err == nil {
			var access database.WhatsAppAccess
			if json.Unmarshal([]byte(cached), &access) == nil {
				return access, nil
			}
		} else if !errors.Is(err, redis.Nil) {
			log.Printf("read WhatsApp access cache: %v", err)
		}
	}

	access, err := h.store.WhatsAppAccess(ctx, phone, now)
	if err != nil {
		return database.WhatsAppAccess{}, err
	}
	if h.redis != nil {
		if encoded, marshalErr := json.Marshal(access); marshalErr == nil {
			if cacheErr := h.redis.Set(ctx, key, encoded, 3*time.Minute).Err(); cacheErr != nil {
				log.Printf("write WhatsApp access cache: %v", cacheErr)
			}
		}
	}
	return access, nil
}

func (h *Handler) sendNotice(ctx context.Context, phone, text string) {
	phone = phoneutil.Normalize(phone)
	if h.wa == nil {
		log.Printf("send WhatsApp notice: WA client not configured")
		return
	}
	if err := h.wa.SendText(ctx, phone, text); err != nil {
		log.Printf("send WhatsApp notice: %v", err)
	}
}
