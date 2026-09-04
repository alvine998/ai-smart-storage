package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"ai-smart-storage/internal/ai"
	"ai-smart-storage/internal/database"
	"ai-smart-storage/internal/http/middleware"
	r2storage "ai-smart-storage/internal/storage"
	service "ai-smart-storage/internal/telegram"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type Handler struct {
	ai        *ai.Client
	store     *database.Store
	tg        *service.Service
	r2        *r2storage.Store
	signupURL string
	redis     *redis.Client
}

const chatSystemPrompt = `You are the AI Smart Storage assistant on Telegram. You help users store and find their files.
- Users upload files by sending a document or photo to this chat.
- To search files, users send "cari file <name>", "cari gambar <name>" or "find my <name>"; the bot replies with matching files.
- Slash commands: /sync (link account), /link <email>, /balance (storage and quota usage).
- If a user asks how to do something, explain the relevant command briefly. Reply in the user's language and keep answers short.`

func New(aiClient *ai.Client, store *database.Store, tg *service.Service, r2 *r2storage.Store, signupURL string, redisClient *redis.Client) *Handler {
	return &Handler{ai: aiClient, store: store, tg: tg, r2: r2, signupURL: signupURL, redis: redisClient}
}

func (h *Handler) Register(app fiber.Router) {
	app.Post("/webhooks/telegram", limiter.New(limiter.Config{Max: 120, Expiration: time.Minute}), h.Receive)
}

func (h *Handler) RegisterProtected(app fiber.Router) {
	app.Get("/v1/tg-conversations", h.ListConversations)
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

func (h *Handler) Receive(c *fiber.Ctx) error {
	if h.tg == nil || !h.tg.IsConfigured() {
		log.Printf("telegram: service not configured")
		return fiber.ErrInternalServerError
	}
	if !h.tg.ValidateSecret(c.Get("X-Telegram-Bot-Api-Secret-Token")) {
		log.Printf("telegram: invalid secret token")
		return fiber.ErrForbidden
	}
	var incoming service.Incoming
	if err := json.Unmarshal(c.Body(), &incoming); err != nil {
		log.Printf("telegram: unmarshal error: %v", err)
		return fiber.ErrBadRequest
	}
	if incoming.Message == nil {
		return c.SendStatus(fiber.StatusOK)
	}
	msg := incoming.Message
	if msg.Contact != nil {
		go h.handleContact(msg.Chat.ID, msg.Contact.PhoneNumber)
		return c.SendStatus(fiber.StatusOK)
	}
	if msg.Document != nil {
		go h.handleMedia(msg.Chat.ID, msg.Document.FileID, msg.Document.FileName, msg.Document.FileSize, msg.Caption)
		return c.SendStatus(fiber.StatusOK)
	}
	if len(msg.Photo) > 0 {
		largest := msg.Photo[len(msg.Photo)-1]
		go h.handleMedia(msg.Chat.ID, largest.FileID, fmt.Sprintf("photo_%d.jpg", msg.Date), largest.FileSize, msg.Caption)
		return c.SendStatus(fiber.StatusOK)
	}
	if msg.Text == "" {
		return c.SendStatus(fiber.StatusOK)
	}
	log.Printf("telegram: message from=%d (%s %s) text=%q", msg.From.ID, msg.From.FirstName, msg.From.LastName, msg.Text)
	text := strings.SplitN(msg.Text, " ", 2)[0]
	cmd := strings.SplitN(text, "@", 2)[0]
	if cmd == "/start" {
		go h.handleStart(msg.Chat.ID, msg.From.FirstName)
		return c.SendStatus(fiber.StatusOK)
	}
	if cmd == "/sync" {
		go h.handleSync(msg.Chat.ID)
		return c.SendStatus(fiber.StatusOK)
	}
	if cmd == "/balance" {
		go h.handleCheckBalance(msg.Chat.ID)
		return c.SendStatus(fiber.StatusOK)
	}
	if cmd == "/checkfiles" {
		go h.handleCheckFiles(msg.Chat.ID, msg.From.ID)
		return c.SendStatus(fiber.StatusOK)
	}
	if cmd == "/link" {
		arg := ""
		if strings.HasPrefix(msg.Text, "/link ") {
			arg = strings.TrimSpace(strings.TrimPrefix(msg.Text, "/link"))
		}
		if arg == "" {
			go h.sendNotice(c.Context(), msg.Chat.ID, "Usage: /link your@email.com")
		} else {
			go h.handleLink(msg.Chat.ID, msg.From.ID, arg)
		}
		return c.SendStatus(fiber.StatusOK)
	}
	lower := strings.ToLower(msg.Text)
	if query, ok := extractSearchQuery(lower); ok {
		if query == "" {
			go h.sendNotice(c.Context(), msg.Chat.ID, "Usage: Find my <filename>")
		} else {
			go h.handleFindFile(msg.Chat.ID, msg.From.ID, query)
		}
		return c.SendStatus(fiber.StatusOK)
	}
	go h.reply(msg.Chat.ID, msg.From.ID, msg.Text)
	return c.SendStatus(fiber.StatusOK)
}

func (h *Handler) reply(chatID int64, fromID int64, text string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("telegram reply panic: %v", r)
		}
	}()
	phone := "tg:" + strconv.FormatInt(chatID, 10)
	log.Printf("telegram reply: chat=%d from=%d text=%q", chatID, fromID, text)
	if h.store == nil {
		log.Printf("telegram reply: store not configured")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	access, err := h.telegramAccess(ctx, chatID)
	if errors.Is(err, database.ErrWhatsAppAccessNotFound) {
		h.sendNotice(ctx, chatID, "Number not registered. Sign up: "+h.signupURL)
		return
	}
	if err != nil {
		log.Printf("check Telegram access: %v", err)
		return
	}
	if !access.WithinQuota() {
		h.sendNotice(ctx, chatID, "Your plan has reached its limit or needs renewal. Please renew or upgrade to continue: "+h.signupURL)
		return
	}
	userID := access.UserID
	if err := h.store.SaveMessage(ctx, "", phone, "user", text); err != nil {
		log.Printf("save inbound message: %v", err)
		return
	}
	if err := h.store.CheckQuota(ctx, userID, 0, 0, 1, 0); err != nil {
		if errors.Is(err, database.ErrQuotaExceeded) {
			h.sendNotice(ctx, chatID, "Your AI query quota has been reached. Please upgrade your plan: "+h.signupURL)
			return
		}
		if errors.Is(err, database.ErrWhatsAppAccessNotFound) {
			h.sendNotice(ctx, chatID, "Your subscription has expired. Please renew: "+h.signupURL)
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
	messages := make([]ai.Message, 0, len(history)+1)
	messages = append(messages, ai.Message{Role: "system", Content: chatSystemPrompt})
	for _, item := range history {
		messages = append(messages, ai.Message{Role: item.Role, Content: item.Content})
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
	if h.tg == nil {
		log.Printf("send Telegram message: client not configured")
		return
	}
	if err := h.tg.SendMessage(ctx, chatID, response.String()); err != nil {
		log.Printf("send Telegram message: %v", err)
		return
	}
}

func (h *Handler) telegramAccess(ctx context.Context, chatID int64) (database.WhatsAppAccess, error) {
	key := "telegram:access:" + strconv.FormatInt(chatID, 10)
	if h.redis != nil {
		cached, err := h.redis.Get(ctx, key).Result()
		if err == nil {
			var access database.WhatsAppAccess
			if json.Unmarshal([]byte(cached), &access) == nil {
				return access, nil
			}
		} else if !errors.Is(err, redis.Nil) {
			log.Printf("read Telegram access cache: %v", err)
		}
	}
	access, err := h.store.TelegramAccess(ctx, chatID, time.Now().UTC())
	if err != nil {
		return database.WhatsAppAccess{}, err
	}
	if h.redis != nil {
		if encoded, marshalErr := json.Marshal(access); marshalErr == nil {
			if cacheErr := h.redis.Set(ctx, key, encoded, 3*time.Minute).Err(); cacheErr != nil {
				log.Printf("write Telegram access cache: %v", cacheErr)
			}
		}
	}
	return access, nil
}

func (h *Handler) sendNotice(ctx context.Context, chatID int64, text string) {
	if h.tg == nil {
		log.Printf("send Telegram notice: client not configured")
		return
	}
	if err := h.tg.SendMessage(ctx, chatID, text); err != nil {
		log.Printf("send Telegram notice: %v", err)
	}
}

func (h *Handler) handleLink(chatID int64, fromID int64, email string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("telegram link panic: %v", r)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if email == "" || !strings.Contains(email, "@") {
		h.sendNotice(ctx, chatID, "Invalid email. Usage: /link your@email.com")
		return
	}
	user, err := h.store.UserByEmail(ctx, email)
	if err != nil {
		log.Printf("telegram link: user lookup by email: %v", err)
		h.sendNotice(ctx, chatID, "Account not found. Please sign up first: "+h.signupURL)
		return
	}
	if err := h.store.LinkTelegramChat(ctx, user.ID, chatID); err != nil {
		log.Printf("telegram link: link chat: %v", err)
		h.sendNotice(ctx, chatID, "Failed to link account. Try again later.")
		return
	}
	log.Printf("telegram link: linked chat %d to user %d (%s)", chatID, user.ID, email)
	h.sendNotice(ctx, chatID, "Account linked! You can now use the bot.")
}

func (h *Handler) handleStart(chatID int64, name string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("telegram start panic: %v", r)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if name == "" {
		name = "there"
	}
	existing, err := h.store.UserByTelegramChat(ctx, chatID)
	if err == nil && existing > 0 {
		h.sendNotice(ctx, chatID, "Welcome back, "+name+"!")
		return
	}
	h.sendNotice(ctx, chatID, "Welcome, "+name+"! Send /sync and share your contact to link your account.")
}

func (h *Handler) handleSync(chatID int64) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("telegram sync panic: %v", r)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	existing, err := h.store.UserByTelegramChat(ctx, chatID)
	if err == nil && existing > 0 {
		h.sendNotice(ctx, chatID, "Your account is already linked.")
		return
	}
	if h.tg == nil {
		log.Printf("send Telegram contact request: client not configured")
		return
	}
	if err := h.tg.SendContactRequest(ctx, chatID); err != nil {
		log.Printf("send Telegram contact request: %v", err)
		h.sendNotice(ctx, chatID, "Failed to start sync. Try again later.")
	}
}

func (h *Handler) handleContact(chatID int64, phoneNumber string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("telegram contact panic: %v", r)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if h.tg != nil {
		h.tg.HideKeyboard(ctx, chatID)
	}
	if phoneNumber == "" {
		h.sendNotice(ctx, chatID, "No phone number received. Try /sync again.")
		return
	}
	existing, _ := h.store.UserByTelegramChat(ctx, chatID)
	if existing > 0 {
		h.sendNotice(ctx, chatID, "Your account is already linked.")
		return
	}
	userID, linked, err := h.store.LinkTelegramByPhoneIfNull(ctx, phoneNumber, chatID)
	if err != nil {
		log.Printf("telegram sync: link by phone: %v", err)
		h.sendNotice(ctx, chatID, "Failed to link account. Try again later.")
		return
	}
	if !linked {
		log.Printf("telegram sync: no unlinked user for phone %s", phoneNumber)
		h.sendNotice(ctx, chatID, "Account not found or already linked. Sign up: "+h.signupURL)
		return
	}
	log.Printf("telegram sync: linked chat %d to user %d via phone %s", chatID, userID, phoneNumber)
	h.sendNotice(ctx, chatID, "Account synced! You can now use the bot.")
}

func (h *Handler) handleMedia(chatID int64, fileID, fileName string, fileSize int64, caption string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("telegram media panic: %v", r)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if h.store == nil {
		log.Printf("telegram media: store not configured")
		return
	}
	access, err := h.telegramAccess(ctx, chatID)
	if errors.Is(err, database.ErrWhatsAppAccessNotFound) {
		h.sendNotice(ctx, chatID, "Account not linked. Send /sync to link your account.")
		return
	}
	if err != nil {
		log.Printf("telegram media: check access: %v", err)
		return
	}
	if !access.WithinQuota() {
		h.sendNotice(ctx, chatID, "Your plan has reached its limit or needs renewal. Please renew or upgrade to continue: "+h.signupURL)
		return
	}
	storageGB := float64(fileSize) / math.Pow10(9)
	if err := h.store.CheckQuota(ctx, access.UserID, storageGB, 0, 0, 0); err != nil {
		if errors.Is(err, database.ErrQuotaExceeded) {
			h.sendNotice(ctx, chatID, "Your storage quota has been reached. Please upgrade your plan: "+h.signupURL)
			return
		}
		if errors.Is(err, database.ErrWhatsAppAccessNotFound) {
			h.sendNotice(ctx, chatID, "Your subscription has expired. Please renew: "+h.signupURL)
			return
		}
		log.Printf("telegram media: check storage quota: %v", err)
		return
	}
	if h.tg == nil {
		log.Printf("telegram media: service not configured")
		return
	}
	h.sendNotice(ctx, chatID, "Downloading your file...")
	data, dlName, mime, err := h.tg.DownloadFile(ctx, fileID)
	if err != nil {
		log.Printf("telegram media: download: %v", err)
		h.sendNotice(ctx, chatID, "Failed to download file. Try again.")
		return
	}
	if fileName == "" {
		fileName = dlName
	}
	if h.r2 == nil {
		log.Printf("telegram media: R2 not configured")
		h.sendNotice(ctx, chatID, "Storage not configured. Please contact support.")
		return
	}
	key := fmt.Sprintf("smart-storage/%d/tg/%s/%s", access.UserID, uuid.NewString(), fileName)
	if err := h.r2.Put(ctx, key, bytes.NewReader(data), int64(len(data)), mime); err != nil {
		log.Printf("telegram media: R2 upload: %v", err)
		h.sendNotice(ctx, chatID, "Failed to store file. Try again.")
		return
	}
	doc, err := h.store.CreateDocument(ctx, database.Document{
		UserID:      access.UserID,
		FileName:    fileName,
		R2Key:       key,
		FileSize:    uint64(fileSize),
		MimeType:    mime,
		Summary:     caption,
		Metadata:    "{}",
		UploadedVia: "telegram",
	})
	if err != nil {
		log.Printf("telegram media: create document: %v", err)
		_ = h.r2.Delete(ctx, key)
		h.sendNotice(ctx, chatID, "Failed to save file record. Try again.")
		return
	}
	if err := h.store.IncrementUsageQuota(ctx, access.UserID, fmt.Sprintf("%.6f", storageGB), 0, 0, 0); err != nil {
		log.Printf("telegram media: increment storage usage: %v", err)
	}
	log.Printf("telegram media: stored document %d (user=%d, file=%s, size=%d)", doc.ID, access.UserID, fileName, fileSize)
	h.sendNotice(ctx, chatID, fmt.Sprintf("File saved: %s\nDocument ID: %d", fileName, doc.ID))
}

func (h *Handler) handleFindFile(chatID int64, fromID int64, query string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("telegram find file panic: %v", r)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if h.store == nil {
		h.sendNotice(ctx, chatID, "Storage not available.")
		return
	}
	access, err := h.telegramAccess(ctx, chatID)
	if errors.Is(err, database.ErrWhatsAppAccessNotFound) {
		h.sendNotice(ctx, chatID, "Account not linked. Send /sync to link your account.")
		return
	}
	if err != nil {
		log.Printf("telegram find file: check access: %v", err)
		return
	}
	docs, err := h.store.SearchDocuments(ctx, access.UserID, query, 5)
	if err != nil {
		log.Printf("telegram find file: search: %v", err)
		h.sendNotice(ctx, chatID, "Failed to search files. Try again.")
		return
	}
	if len(docs) == 0 {
		h.sendNotice(ctx, chatID, fmt.Sprintf("No files found matching \"%s\".", query))
		return
	}
	if len(docs) == 1 {
		doc := docs[0]
		if h.r2 == nil || h.tg == nil {
			h.sendNotice(ctx, chatID, fmt.Sprintf("Found: %s (ID: %d)", doc.FileName, doc.ID))
			return
		}
		h.sendNotice(ctx, chatID, fmt.Sprintf("Found: %s\nSending file...", doc.FileName))
		object, err := h.r2.Get(ctx, doc.R2Key)
		if err != nil {
			log.Printf("telegram find file: R2 get: %v", err)
			h.sendNotice(ctx, chatID, fmt.Sprintf("Found: %s (ID: %d)\nFailed to download from storage.", doc.FileName, doc.ID))
			return
		}
		defer object.Close()
		data, err := io.ReadAll(object)
		if err != nil {
			log.Printf("telegram find file: read object: %v", err)
			h.sendNotice(ctx, chatID, fmt.Sprintf("Found: %s (ID: %d)\nFailed to read file.", doc.FileName, doc.ID))
			return
		}
		if err := h.tg.SendDocument(ctx, chatID, doc.FileName, data, doc.MimeType, ""); err != nil {
			log.Printf("telegram find file: send document: %v", err)
			h.sendNotice(ctx, chatID, fmt.Sprintf("Found: %s (ID: %d)\nFailed to send file.", doc.FileName, doc.ID))
			return
		}
		return
	}
	var list strings.Builder
	list.WriteString(fmt.Sprintf("Found %d files:\n", len(docs)))
	for i, doc := range docs {
		sizeMB := float64(doc.FileSize) / (1024 * 1024)
		list.WriteString(fmt.Sprintf("%d. %s (%.1fMB, ID: %d)\n", i+1, doc.FileName, sizeMB, doc.ID))
	}
	list.WriteString("\nSend the exact filename to download it.")
	h.sendNotice(ctx, chatID, list.String())
}

func (h *Handler) handleCheckBalance(chatID int64) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("telegram check-balance panic: %v", r)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if h.store == nil {
		h.sendNotice(ctx, chatID, "Storage not available.")
		return
	}
	access, err := h.telegramAccess(ctx, chatID)
	if errors.Is(err, database.ErrWhatsAppAccessNotFound) {
		h.sendNotice(ctx, chatID, "Account not linked. Send /sync to link your account.")
		return
	}
	if err != nil {
		log.Printf("telegram check-balance: access: %v", err)
		h.sendNotice(ctx, chatID, "Failed to check balance. Try again.")
		return
	}
	storageUsed, _ := strconv.ParseFloat(access.StorageUsedGB, 64)
	storageLimit, _ := strconv.ParseFloat(access.StorageLimitGB, 64)
	storagePct := float64(0)
	if storageLimit > 0 {
		storagePct = (storageUsed / storageLimit) * 100
	}
	periodEnd := "N/A"
	if !access.PeriodEnd.IsZero() {
		periodEnd = access.PeriodEnd.Format("2006-01-02")
	}
	var msg strings.Builder
	msg.WriteString("📊 Account Balance\n\n")
	msg.WriteString(fmt.Sprintf("Status: %s\n", access.Status))
	msg.WriteString(fmt.Sprintf("Period ends: %s\n\n", periodEnd))
	msg.WriteString(fmt.Sprintf("Storage: %.2f / %.2f GB (%.0f%%)\n", storageUsed, storageLimit, storagePct))
	msg.WriteString(fmt.Sprintf("AI Queries: %d / %d\n", access.AIQueriesUsed, access.AIQueryLimit))
	msg.WriteString(fmt.Sprintf("AI Docs: %d / %d\n", access.AIDocsUsed, access.AIDocsLimit))
	if access.InGracePeriod(time.Now().UTC()) {
		msg.WriteString("\n⚠️ Subscription expired. Please renew.")
	}
	h.sendNotice(ctx, chatID, msg.String())
}

func (h *Handler) handleCheckFiles(chatID int64, fromID int64) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("telegram check-files panic: %v", r)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if h.store == nil {
		h.sendNotice(ctx, chatID, "Storage not available.")
		return
	}
	access, err := h.telegramAccess(ctx, chatID)
	if errors.Is(err, database.ErrWhatsAppAccessNotFound) {
		h.sendNotice(ctx, chatID, "Account not linked. Send /sync to link your account.")
		return
	}
	if err != nil {
		log.Printf("telegram check-files: check access: %v", err)
		return
	}
	docs, err := h.store.Documents(ctx, access.UserID, 50, 0)
	if err != nil {
		log.Printf("telegram check-files: list documents: %v", err)
		h.sendNotice(ctx, chatID, "Failed to load your files. Try again.")
		return
	}
	if len(docs) == 0 {
		h.sendNotice(ctx, chatID, "You have no files stored yet. Send a document or photo to store it, or search with: cari file <name>")
		return
	}
	var list strings.Builder
	list.WriteString(fmt.Sprintf("You have %d file(s):\n", len(docs)))
	for i, doc := range docs {
		sizeMB := float64(doc.FileSize) / (1024 * 1024)
		list.WriteString(fmt.Sprintf("%d. %s (%.1fMB, ID: %d)\n", i+1, doc.FileName, sizeMB, doc.ID))
	}
	list.WriteString("\nTo download one, send the exact filename. Search with: cari file <name>")
	h.sendNotice(ctx, chatID, list.String())
}

// extractSearchQuery detects file-search requests. The search phrase must
// start the message, so questions like "kalau aku mau cari file bagaimana
// caranya?" fall through to the AI chat instead of running a bogus search.
func extractSearchQuery(text string) (string, bool) {
	q := ""
	switch {
	// "find my <query> file" or "find my <query>"
	case strings.HasPrefix(text, "find my "):
		q = strings.TrimPrefix(text, "find my ")
	// "cari file atau gambar dengan nama <query>"
	case strings.HasPrefix(text, "cari file") || strings.HasPrefix(text, "cari gambar"):
		if idx := strings.LastIndex(text, "nama "); idx >= 0 {
			return strings.TrimSpace(text[idx+5:]), true
		}
		// "cari file <query>", "cari gambar <query>", "cari file atau gambar <query>"
		q = strings.TrimPrefix(text, "cari ")
		q = strings.TrimPrefix(q, "file ")
		q = strings.TrimPrefix(q, "gambar ")
		q = strings.TrimPrefix(q, "atau gambar ")
	// "cari <query>"
	case strings.HasPrefix(text, "cari "):
		q = strings.TrimPrefix(text, "cari ")
	default:
		return "", false
	}
	q = strings.TrimSpace(q)
	// Drop a trailing noun ("find my report file", "cari laporan gambar").
	q = strings.TrimSpace(strings.TrimSuffix(q, " file"))
	q = strings.TrimSpace(strings.TrimSuffix(q, " gambar"))
	if q == "" || q == "file" || q == "gambar" {
		return "", true
	}
	return q, true
}
