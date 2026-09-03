package chat

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"ai-smart-storage/internal/ai"
	"ai-smart-storage/internal/database"
	"ai-smart-storage/internal/http/middleware"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
)

type Handler struct {
	client     *ai.Client
	store      *database.Store
	inputCost  float64
	outputCost float64
}

func New(client *ai.Client, store *database.Store, inputCost, outputCost float64) *Handler {
	return &Handler{client: client, store: store, inputCost: inputCost, outputCost: outputCost}
}

func (h *Handler) Register(app fiber.Router) {
	app.Post("/v1/chat/stream", limiter.New(limiter.Config{
		Max:        10,
		Expiration: time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return strconv.FormatUint(middleware.UserID(c), 10)
		},
	}), h.Stream)
}

func (h *Handler) Stream(c *fiber.Ctx) error {
	var input struct {
		UserID   uint64       `json:"user_id"`
		Messages []ai.Message `json:"messages"`
	}
	if err := c.BodyParser(&input); err != nil || len(input.Messages) == 0 {
		return fiber.ErrBadRequest
	}
	userID := middleware.UserID(c)
	if input.UserID != 0 && input.UserID != userID {
		return fiber.NewError(fiber.StatusForbidden, "user_id does not match the authenticated user")
	}
	if h.store != nil {
		if err := h.store.CheckQuota(c.Context(), userID, 0, 0, 1, 0); err != nil {
			if errors.Is(err, database.ErrQuotaExceeded) {
				return fiber.NewError(fiber.StatusTooManyRequests, "AI query quota exceeded")
			}
			if errors.Is(err, database.ErrWhatsAppAccessNotFound) {
				return fiber.NewError(fiber.StatusForbidden, "subscription required or expired")
			}
			return fiber.ErrInternalServerError
		}
	}
	c.Set(fiber.HeaderContentType, "text/event-stream")
	c.Set(fiber.HeaderCacheControl, "no-cache")
	c.Set(fiber.HeaderConnection, "keep-alive")
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		if h.client == nil {
			_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", jsonString("AI client not configured"))
			_ = w.Flush()
			return
		}
		usage, err := h.client.StreamWithUsage(context.Background(), input.Messages, func(part string) error {
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
		cost := float64(usage.InputTokens)/1000*h.inputCost + float64(usage.OutputTokens)/1000*h.outputCost
		if h.store != nil {
			if err := h.store.CreateAIProcessingLog(context.Background(), database.AIProcessingLog{UserID: userID, ActionType: "search_query", InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens, EstimatedCost: fmt.Sprintf("%.8f", cost)}); err != nil {
				fmt.Fprintf(w, "event: log_error\ndata: %s\n\n", jsonString(err.Error()))
				_ = w.Flush()
			}
			if err := h.store.IncrementUsageQuota(context.Background(), userID, "0", 0, 1, 0); err != nil {
				fmt.Fprintf(w, "event: log_error\ndata: %s\n\n", jsonString(err.Error()))
				_ = w.Flush()
			}
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
