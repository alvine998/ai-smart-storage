package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"strings"
	"time"

	"ai-smart-storage/internal/ai"
	"ai-smart-storage/internal/database"
	"ai-smart-storage/internal/filecommands"
	"ai-smart-storage/internal/http/middleware"
	phoneutil "ai-smart-storage/internal/phone"
	r2storage "ai-smart-storage/internal/storage"
	service "ai-smart-storage/internal/whatsapp"

	"github.com/google/uuid"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/redis/go-redis/v9"
)

type Handler struct {
	ai        *ai.Client
	store     *database.Store
	wa        *service.Service
	r2        *r2storage.Store
	signupURL string
	cacheTTL  time.Duration
	redis     *redis.Client
}

func New(aiClient *ai.Client, store *database.Store, wa *service.Service, r2 *r2storage.Store, signupURL string, redisClient *redis.Client) *Handler {
	return &Handler{ai: aiClient, store: store, wa: wa, r2: r2, signupURL: signupURL, redis: redisClient}
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
	messageCount := 0
	for _, entry := range incoming.Entry {
		for _, change := range entry.Changes {
			for _, message := range change.Value.Messages {
				messageCount++
				log.Printf("whatsapp: message type=%s from=%s id=%s", message.Type, message.From, message.ID)
				switch message.Type {
				case "text":
					go h.reply(message.ID, message.From, message.Text.Body)
				case "document":
					if message.Document != nil {
						go h.handleMedia(message.ID, message.From, message.Document.ID, message.Document.Filename, message.Document.MimeType, message.Document.Caption, false)
					}
				case "image":
					if message.Image != nil {
						go h.handleMedia(message.ID, message.From, message.Image.ID, "", message.Image.MimeType, message.Image.Caption, true)
					}
				}
			}
		}
	}
	if messageCount == 0 {
		log.Printf("whatsapp: received webhook with %d entries but no messages (likely status update)", len(incoming.Entry))
	}
	return c.SendStatus(fiber.StatusOK)
}

func (h *Handler) handleMedia(id, phone, mediaID, fileName, mimeType, caption string, image bool) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("whatsapp media panic: %v", r)
		}
	}()
	phone = phoneutil.Normalize(phone)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if h.store == nil || h.wa == nil || h.r2 == nil {
		log.Printf("whatsapp media: service not configured")
		return
	}
	access, err := h.whatsAppAccess(ctx, phone)
	if errors.Is(err, database.ErrWhatsAppAccessNotFound) {
		h.sendNotice(ctx, phone, "Number not registered. Sign up: "+h.signupURL)
		return
	}
	if err != nil {
		log.Printf("whatsapp media: access: %v", err)
		return
	}
	if !access.WithinQuota() {
		h.sendNotice(ctx, phone, "Your plan has reached its limit or needs renewal. Please renew or upgrade to continue: "+h.signupURL)
		return
	}
	_ = h.store.LogWAConversation(ctx, database.WAConversation{UserID: access.UserID, WAMessageID: id, Direction: "inbound", MessageType: "media", Category: "service", Content: fileName, Cost: "0"})
	if err := h.store.IncrementUsageQuota(ctx, access.UserID, "0", 0, 0, 1); err != nil {
		log.Printf("whatsapp media: increment inbound usage: %v", err)
	}
	if err := h.store.OpenWAWindow(ctx, access.UserID, time.Now().UTC()); err != nil {
		log.Printf("whatsapp media: open window: %v", err)
	}
	data, downloadedMime, err := h.wa.DownloadMedia(ctx, mediaID)
	if err != nil {
		log.Printf("whatsapp media: download: %v", err)
		h.sendNotice(ctx, phone, "Failed to download media. Try again.")
		return
	}
	if mimeType == "" {
		mimeType = downloadedMime
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	if fileName == "" {
		ext := "bin"
		if image {
			ext = "jpg"
		}
		fileName = fmt.Sprintf("whatsapp_%d.%s", time.Now().UnixNano(), ext)
	}
	storageGB := float64(len(data)) / math.Pow10(9)
	if err := h.store.CheckQuota(ctx, access.UserID, storageGB, 0, 0, 0); err != nil {
		if errors.Is(err, database.ErrQuotaExceeded) {
			h.sendNotice(ctx, phone, "Your storage quota has been reached. Please upgrade your plan: "+h.signupURL)
		} else {
			log.Printf("whatsapp media: storage quota: %v", err)
		}
		return
	}
	key := fmt.Sprintf("smart-storage/%d/wa/%s/%s", access.UserID, uuid.NewString(), fileName)
	if err := h.r2.Put(ctx, key, bytes.NewReader(data), int64(len(data)), mimeType); err != nil {
		log.Printf("whatsapp media: R2 upload: %v", err)
		h.sendNotice(ctx, phone, "Failed to store media. Try again.")
		return
	}
	if _, err := h.store.CreateDocument(ctx, database.Document{UserID: access.UserID, FileName: fileName, R2Key: key, FileSize: uint64(len(data)), MimeType: mimeType, Summary: caption, Metadata: "{}", UploadedVia: "whatsapp"}); err != nil {
		_ = h.r2.Delete(ctx, key)
		log.Printf("whatsapp media: create document: %v", err)
		h.sendNotice(ctx, phone, "Failed to save media record. Try again.")
		return
	}
	if err := h.store.IncrementUsageQuota(ctx, access.UserID, fmt.Sprintf("%.6f", storageGB), 0, 0, 0); err != nil {
		log.Printf("whatsapp media: increment storage usage: %v", err)
	}
	h.sendNotice(ctx, phone, "File saved: "+fileName)
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
	if command, ok, parseErr := filecommands.Parse(text); ok {
		if parseErr != nil {
			h.sendNotice(ctx, phone, "Usage: kirim semua file, kirim semua foto, atau kirim file 1,2,3")
			return
		}
		if command.All || len(command.Positions) > 0 || command.Query != "" {
			h.handleBatchFiles(ctx, phone, userID, command)
			return
		}
		h.sendNotice(ctx, phone, "Usage: kirim semua file, kirim semua foto, atau kirim file 1,2,3")
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

func (h *Handler) handleBatchFiles(ctx context.Context, phone string, userID uint64, command filecommands.Command) {
	if h.store == nil || h.wa == nil || h.r2 == nil {
		h.sendNotice(ctx, phone, "Storage is not available.")
		return
	}
	selection, err := filecommands.SelectWithMissing(ctx, h.store, userID, command)
	if err != nil {
		log.Printf("whatsapp batch files: select: %v", err)
		h.sendNotice(ctx, phone, "Failed to select files. Try again.")
		return
	}
	if len(selection.Documents) == 0 {
		if len(selection.Missing) > 0 {
			h.sendNotice(ctx, phone, fmt.Sprintf("File positions not found: %v", selection.Missing))
		} else {
			h.sendNotice(ctx, phone, "No matching files found.")
		}
		return
	}
	h.sendNotice(ctx, phone, fmt.Sprintf("Sending %d file(s)...", len(selection.Documents)))
	sent, failed := 0, len(selection.Missing)
	for _, document := range selection.Documents {
		if err := h.sendStoredDocument(ctx, phone, userID, document); err != nil {
			failed++
			log.Printf("whatsapp batch files: send %s: %v", document.FileName, err)
			continue
		}
		sent++
	}
	h.sendNotice(ctx, phone, fmt.Sprintf("Finished: %d sent, %d failed.", sent, failed))
}

func (h *Handler) sendStoredDocument(ctx context.Context, phone string, userID uint64, document database.Document) error {
	if err := h.store.CheckQuota(ctx, userID, 0, 0, 0, 1); err != nil {
		return fmt.Errorf("outbound quota: %w", err)
	}
	object, err := h.r2.Get(ctx, document.R2Key)
	if err != nil {
		return fmt.Errorf("R2 get: %w", err)
	}
	defer object.Close()
	data, err := io.ReadAll(object)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	if err := h.wa.SendMedia(ctx, phone, document.FileName, document.MimeType, data, strings.HasPrefix(strings.ToLower(document.MimeType), "image/")); err != nil {
		return fmt.Errorf("send media: %w", err)
	}
	open, err := h.store.WAWindowOpen(ctx, userID, time.Now().UTC())
	if err != nil {
		log.Printf("whatsapp batch files: check window: %v", err)
	}
	category := "utility"
	if open {
		category = "service"
	}
	_ = h.store.LogWAConversation(ctx, database.WAConversation{UserID: userID, Direction: "outbound", MessageType: "media", Category: category, Content: document.FileName, Cost: "0"})
	if err := h.store.IncrementUsageQuota(ctx, userID, "0", 0, 0, 1); err != nil {
		return fmt.Errorf("increment outbound quota: %w", err)
	}
	return nil
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
