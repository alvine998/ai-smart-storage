package health

import (
	"context"
	"time"

	"ai-smart-storage/internal/ai"
	"ai-smart-storage/internal/config"
	"ai-smart-storage/internal/database"
	"ai-smart-storage/internal/diagnostics"
	"ai-smart-storage/internal/storage"
	"ai-smart-storage/internal/whatsapp"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

type Handler struct {
	store *database.Store
	r2    *storage.Store
	redis *redis.Client
	ai    *ai.Client
	wa    *whatsapp.Service
	cfg   config.Config
}

// New keeps backward compatibility (DB + R2 only).
func New(store *database.Store, r2 *storage.Store) *Handler {
	return &Handler{store: store, r2: r2}
}

// NewWithDiagnostics wires all 5 external dependencies for full health reporting.
func NewWithDiagnostics(store *database.Store, r2 *storage.Store, redisClient *redis.Client, aiClient *ai.Client, wa *whatsapp.Service, cfg config.Config) *Handler {
	return &Handler{store: store, r2: r2, redis: redisClient, ai: aiClient, wa: wa, cfg: cfg}
}

func (h *Handler) Register(app fiber.Router) {
	app.Get("/health", h.Health)
	app.Get("/health/diagnostics", h.Diagnostics)
}

type HealthResponse struct {
	Status    string `json:"status"`
	Database  string `json:"database"`
	Storage   string `json:"storage"`
	Redis     string `json:"redis"`
	WhatsApp  string `json:"whatsapp"`
	Mimo      string `json:"mimo"`
	Details   []diagnostics.Result `json:"details,omitempty"`
	CheckedAt string               `json:"checked_at"`
}

func (h *Handler) Health(c *fiber.Ctx) error {
	// If diagnostics stack is wired, reuse it for a single unified check.
	if h.redis != nil || h.ai != nil || h.wa != nil || h.cfg.RedisURL != "" {
		return h.Diagnostics(c)
	}

	// Legacy fallback: only DB + R2 (keeps existing tests green without extra deps)
	resp := HealthResponse{Status: "ok", CheckedAt: time.Now().UTC().Format(time.RFC3339)}

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
	resp.Redis = "unknown"
	resp.WhatsApp = "unknown"
	resp.Mimo = "unknown"

	statusCode := fiber.StatusOK
	if resp.Status == "degraded" {
		statusCode = fiber.StatusServiceUnavailable
	}
	return c.Status(statusCode).JSON(resp)
}

// Diagnostics runs concurrent probes for all 5 dependencies and returns detailed results.
func (h *Handler) Diagnostics(c *fiber.Ctx) error {
	checker := diagnostics.New(h.cfg, h.store, h.r2, h.ai, h.wa, h.redis)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	results := checker.CheckAll(ctx)

	resp := HealthResponse{
		Status:    diagnostics.OverallStatus(results),
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
		Details:   results,
	}
	// Map to legacy top-level fields for backward compatibility
	for _, r := range results {
		val := r.Status
		if r.Error != "" {
			val = "error: " + r.Error
		} else if r.Detail != "" && r.Status == "ok" {
			val = "ok (" + r.Detail + ")"
		}
		switch r.Name {
		case "DB (MySQL)":
			resp.Database = val
		case "Redis":
			resp.Redis = val
		case "Cloudflare R2":
			resp.Storage = val
		case "WhatsApp API":
			resp.WhatsApp = val
		case "MiMo V2.5":
			resp.Mimo = val
		}
	}
	// Ensure no empty fields
	if resp.Database == "" {
		resp.Database = "unknown"
	}
	if resp.Storage == "" {
		resp.Storage = "unknown"
	}
	if resp.Redis == "" {
		resp.Redis = "unknown"
	}
	if resp.WhatsApp == "" {
		resp.WhatsApp = "unknown"
	}
	if resp.Mimo == "" {
		resp.Mimo = "unknown"
	}

	statusCode := fiber.StatusOK
	if resp.Status == "degraded" {
		statusCode = fiber.StatusServiceUnavailable
	}
	return c.Status(statusCode).JSON(resp)
}
