package ai_logs

import (
	"ai-smart-storage/internal/database"
	"ai-smart-storage/internal/http/middleware"

	"github.com/gofiber/fiber/v2"
)

type Handler struct{ store *database.Store }

func New(store *database.Store) *Handler { return &Handler{store: store} }

func (h *Handler) Register(app fiber.Router) { app.Get("/v1/ai-processing-logs", h.List) }

func (h *Handler) List(c *fiber.Ctx) error {
	userID, err := middleware.SelfID(c, c.Query("user_id"))
	if err != nil {
		return err
	}
	limit := 20
	if l := c.QueryInt("limit", 20); l > 0 && l <= 100 {
		limit = l
	}
	offset := c.QueryInt("offset", 0)
	if offset < 0 {
		offset = 0
	}
	items, err := h.store.AIProcessingLogs(c.Context(), userID, limit, offset)
	if err != nil {
		return fiber.ErrInternalServerError
	}
	return c.JSON(items)
}
