package ai_logs

import (
	"strconv"

	"ai-smart-storage/internal/database"
	"github.com/gofiber/fiber/v2"
)

type Handler struct{ store *database.Store }

func New(store *database.Store) *Handler { return &Handler{store: store} }

func (h *Handler) Register(app fiber.Router) { app.Get("/v1/ai-processing-logs", h.List) }

func (h *Handler) List(c *fiber.Ctx) error {
	userID, err := strconv.ParseUint(c.Query("user_id"), 10, 64)
	if err != nil || userID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "user_id is required")
	}
	items, err := h.store.AIProcessingLogs(c.Context(), userID)
	if err != nil {
		return fiber.ErrInternalServerError
	}
	return c.JSON(items)
}
