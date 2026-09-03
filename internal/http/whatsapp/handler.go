package whatsapp

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"ai-smart-storage/internal/ai"
	"ai-smart-storage/internal/database"
	service "ai-smart-storage/internal/whatsapp"
	"github.com/gofiber/fiber/v2"
)

type Handler struct {
	ai    *ai.Client
	store *database.Store
	wa    *service.Service
}

func New(aiClient *ai.Client, store *database.Store, wa *service.Service) *Handler {
	return &Handler{ai: aiClient, store: store, wa: wa}
}

func (h *Handler) Register(app fiber.Router) {
	app.Get("/webhooks/whatsapp", h.Verify)
	app.Post("/webhooks/whatsapp", h.Receive)
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
	if err := h.store.SaveMessage(ctx, id, phone, "user", text); err != nil {
		log.Printf("save inbound message: %v", err)
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
	if err := h.store.SaveMessage(ctx, "", phone, "assistant", response.String()); err != nil {
		log.Printf("save AI message: %v", err)
		return
	}
	if err := h.wa.SendText(ctx, phone, response.String()); err != nil {
		log.Printf("send WhatsApp message: %v", err)
	}
}
