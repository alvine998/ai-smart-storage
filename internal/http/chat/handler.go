package chat

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"ai-smart-storage/internal/ai"
	"ai-smart-storage/internal/database"

	"github.com/gofiber/fiber/v2"
)

type Handler struct {
	client *ai.Client
	quota  *database.Store
}

func New(client *ai.Client, quota *database.Store) *Handler {
	return &Handler{client: client, quota: quota}
}

func (h *Handler) Register(app fiber.Router) { app.Post("/v1/chat/stream", h.Stream) }

func (h *Handler) Stream(c *fiber.Ctx) error {
	var input struct {
		Messages []ai.Message `json:"messages"`
	}
	if err := c.BodyParser(&input); err != nil || len(input.Messages) == 0 {
		return fiber.ErrBadRequest
	}
	userID, err := strconv.ParseUint(c.Get("X-User-ID"), 10, 64)
	if err != nil || userID == 0 {
		return fiber.ErrUnauthorized
	}
	var estimatedTokens uint64
	for _, message := range input.Messages {
		estimatedTokens += uint64((len(message.Content) + 3) / 4)
	}
	const maxOutputTokens = 2048
	if err := h.quota.ReserveAITokens(c.Context(), userID, estimatedTokens+maxOutputTokens); err == database.ErrQuotaExceeded {
		return fiber.NewError(fiber.StatusTooManyRequests, "AI token quota exceeded")
	} else if err != nil {
		return fiber.ErrInternalServerError
	}
	c.Set(fiber.HeaderContentType, "text/event-stream")
	c.Set(fiber.HeaderCacheControl, "no-cache")
	c.Set(fiber.HeaderConnection, "keep-alive")
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		err := h.client.Stream(context.Background(), input.Messages, func(part string) error {
			encoded, err := json.Marshal(part)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", encoded); err != nil {
				return err
			}
			return w.Flush()
		})
		if err != nil {
			_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", jsonString(err.Error()))
			_ = w.Flush()
			return
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		_ = w.Flush()
	})
	return nil
}

func jsonString(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
