package storage

import (
	"errors"
	"fmt"
	"io"
	"math"
	"path"
	"strconv"
	"strings"
	"time"

	"ai-smart-storage/internal/database"
	"ai-smart-storage/internal/http/middleware"
	"ai-smart-storage/internal/storage"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
)

type Handler struct {
	store *storage.Store
	db    *database.Store
}

func New(store *storage.Store) *Handler { return &Handler{store: store} }

// NewWithDB creates a handler with quota enforcement via the database store.
func NewWithDB(store *storage.Store, db *database.Store) *Handler {
	return &Handler{store: store, db: db}
}

func (h *Handler) Register(app fiber.Router) {
	app.Post("/v1/storage/upload", limiter.New(limiter.Config{
		Max:        20,
		Expiration: time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return strconv.FormatUint(middleware.UserID(c), 10)
		},
	}), h.Upload)
	app.Get("/v1/storage/*", h.Download)
}

func (h *Handler) Upload(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return fiber.ErrBadRequest
	}
	requested := c.FormValue("key")
	if requested == "" {
		requested = file.Filename
	}
	userID := middleware.UserID(c)
	key := ScopedKey(userID, requested)
	if h.db != nil {
		storageGB := float64(file.Size) / math.Pow10(9)
		if err := h.db.CheckQuota(c.Context(), userID, storageGB, 0, 0, 0); err != nil {
			if errors.Is(err, database.ErrQuotaExceeded) {
				return fiber.NewError(fiber.StatusTooManyRequests, "storage quota exceeded")
			}
			if errors.Is(err, database.ErrWhatsAppAccessNotFound) {
				return fiber.NewError(fiber.StatusForbidden, "subscription required or expired")
			}
			return fiber.ErrInternalServerError
		}
	}
	opened, err := file.Open()
	if err != nil {
		return fiber.ErrBadRequest
	}
	defer opened.Close()
	if h.store == nil {
		return fiber.ErrInternalServerError
	}
	if err := h.store.Put(c.Context(), key, opened, file.Size, file.Header.Get("Content-Type")); err != nil {
		return fiber.ErrInternalServerError
	}
	if h.db != nil {
		if err := h.db.IncrementUsageQuota(c.Context(), userID, fmt.Sprintf("%.6f", float64(file.Size)/math.Pow10(9)), 0, 0, 0); err != nil {
			// log but do not fail the upload
		}
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"key": key, "url": h.store.PublicURL(key)})
}

func (h *Handler) Download(c *fiber.Ctx) error {
	if h.store == nil {
		return fiber.ErrNotFound
	}
	key := ScopedKey(middleware.UserID(c), c.Params("*"))
	object, err := h.store.Get(c.Context(), key)
	if err != nil {
		return fiber.ErrNotFound
	}
	defer object.Close()
	c.Set(fiber.HeaderContentType, storage.SafeContentType(object.ContentType))
	c.Set(fiber.HeaderContentDisposition, fmt.Sprintf(`attachment; filename="%s"`, path.Base(key)))
	c.Set(fiber.HeaderXContentTypeOptions, "nosniff")
	_, err = io.Copy(c, object)
	return err
}

func ScopedKey(userID uint64, requested string) string {
	cleaned := path.Clean("/" + strings.TrimLeft(requested, "/"))
	return "uploads/" + strconv.FormatUint(userID, 10) + cleaned
}
