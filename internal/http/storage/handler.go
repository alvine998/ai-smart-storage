package storage

import (
	"io"
	"strconv"

	"ai-smart-storage/internal/database"
	"ai-smart-storage/internal/storage"

	"github.com/gofiber/fiber/v2"
)

type Handler struct {
	store *storage.Store
	quota *database.Store
}

func New(store *storage.Store, quota *database.Store) *Handler {
	return &Handler{store: store, quota: quota}
}

func (h *Handler) Register(app fiber.Router) {
	app.Post("/v1/storage/upload", h.Upload)
	app.Get("/v1/storage/*", h.Download)
}

func (h *Handler) Upload(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return fiber.ErrBadRequest
	}
	key := c.FormValue("key")
	if key == "" {
		key = file.Filename
	}
	userID, err := strconv.ParseUint(c.Get("X-User-ID"), 10, 64)
	if err != nil || userID == 0 {
		return fiber.ErrUnauthorized
	}
	if err := h.quota.ReserveStorage(c.Context(), userID, uint64(file.Size)); err == database.ErrQuotaExceeded {
		return fiber.NewError(fiber.StatusTooManyRequests, "storage quota exceeded")
	} else if err != nil {
		return fiber.ErrInternalServerError
	}
	opened, err := file.Open()
	if err != nil {
		return fiber.ErrBadRequest
	}
	defer opened.Close()
	if err := h.store.Put(c.Context(), key, opened, file.Size, file.Header.Get("Content-Type")); err != nil {
		return fiber.ErrInternalServerError
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"key": key, "url": h.store.PublicURL(key)})
}

func (h *Handler) Download(c *fiber.Ctx) error {
	object, err := h.store.Get(c.Context(), c.Params("*"))
	if err != nil {
		return fiber.ErrNotFound
	}
	defer object.Close()
	c.Set(fiber.HeaderContentType, object.ContentType)
	_, err = io.Copy(c, object)
	return err
}
