package health

import (
	"context"
	"time"

	"ai-smart-storage/internal/database"
	"ai-smart-storage/internal/storage"

	"github.com/gofiber/fiber/v2"
)

type Handler struct {
	store *database.Store
	r2    *storage.Store
}

func New(store *database.Store, r2 *storage.Store) *Handler {
	return &Handler{store: store, r2: r2}
}

func (h *Handler) Register(app fiber.Router) {
	app.Get("/health", h.Health)
}

type HealthResponse struct {
	Status   string `json:"status"`
	Database string `json:"database"`
	Storage  string `json:"storage"`
}

func (h *Handler) Health(c *fiber.Ctx) error {
	resp := HealthResponse{Status: "ok"}

	// Probe MySQL with timeout
	if h.store != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := h.store.Ping(ctx)
		cancel()
		if err != nil {
			resp.Database = "error: " + err.Error()
			resp.Status = "degraded"
		} else {
			resp.Database = "ok"
		}
	} else {
		resp.Database = "unknown"
	}

	// Probe R2 with a simple bucket list
	if h.r2 != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := h.r2.Ping(ctx)
		cancel()
		if err != nil {
			resp.Storage = "error: " + err.Error()
			resp.Status = "degraded"
		} else {
			resp.Storage = "ok"
		}
	} else {
		resp.Storage = "unknown"
	}

	statusCode := fiber.StatusOK
	if resp.Status == "degraded" {
		statusCode = fiber.StatusServiceUnavailable
	}
	return c.Status(statusCode).JSON(resp)
}
