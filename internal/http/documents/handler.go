package documents

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"ai-smart-storage/internal/database"
	"ai-smart-storage/internal/http/middleware"
	r2storage "ai-smart-storage/internal/storage"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/google/uuid"
)

type Handler struct {
	store *database.Store
	r2    *r2storage.Store
}

func New(store *database.Store, r2 *r2storage.Store) *Handler { return &Handler{store: store, r2: r2} }

func (h *Handler) Register(app fiber.Router) {
	app.Post("/v1/documents", limiter.New(limiter.Config{
		Max:        20,
		Expiration: time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return strconv.FormatUint(middleware.UserID(c), 10)
		},
	}), h.Create)
	app.Get("/v1/documents", h.List)
	app.Get("/v1/documents/:id", h.Get)
	app.Put("/v1/documents/:id", h.Update)
	app.Delete("/v1/documents/:id", h.Delete)
	app.Get("/v1/documents/:id/download", h.Download)
	app.Get("/v1/documents/:id/versions", h.Versions)
	app.Post("/v1/documents/:id/tags", h.CreateTag)
	app.Get("/v1/documents/:id/tags", h.ListTags)
	app.Delete("/v1/documents/:id/tags/:tagID", h.DeleteTag)
}

func (h *Handler) Create(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return fiber.ErrBadRequest
	}
	userID, err := middleware.SelfID(c, c.FormValue("user_id"))
	if err != nil {
		return err
	}
	uploadedVia := c.FormValue("uploaded_via")
	if uploadedVia == "" {
		uploadedVia = "web"
	}
	if uploadedVia != "web" && uploadedVia != "whatsapp" {
		return fiber.NewError(fiber.StatusBadRequest, "uploaded_via must be web or whatsapp")
	}
	if h.store != nil {
		storageGB := float64(file.Size) / math.Pow10(9)
		if err := h.store.CheckQuota(c.Context(), userID, storageGB, 0, 0, 0); err != nil {
			if errors.Is(err, database.ErrQuotaExceeded) {
				return fiber.NewError(fiber.StatusTooManyRequests, "storage quota exceeded")
			}
			if errors.Is(err, database.ErrWhatsAppAccessNotFound) {
				return fiber.NewError(fiber.StatusForbidden, "subscription required or expired")
			}
			return fiber.ErrInternalServerError
		}
	}
	key := fmt.Sprintf("smart-storage/%d/%s/%s", userID, uuid.NewString(), sanitizeName(file.Filename))
	opened, err := file.Open()
	if err != nil {
		return fiber.ErrBadRequest
	}
	defer opened.Close()
	if err := h.r2.Put(c.Context(), key, opened, file.Size, file.Header.Get("Content-Type")); err != nil {
		return fiber.ErrInternalServerError
	}
	metadata := c.FormValue("metadata")
	if metadata == "" {
		metadata = "{}"
	}
	if !json.Valid([]byte(metadata)) {
		_ = h.r2.Delete(c.Context(), key)
		return fiber.NewError(fiber.StatusBadRequest, "metadata must be valid JSON")
	}
	document, err := h.store.CreateDocument(c.Context(), database.Document{UserID: userID, FileName: file.Filename, R2Key: key, FileSize: uint64(file.Size), MimeType: file.Header.Get("Content-Type"), Category: c.FormValue("category"), Summary: c.FormValue("summary"), Metadata: metadata, UploadedVia: uploadedVia})
	if err != nil {
		_ = h.r2.Delete(c.Context(), key)
		return fiber.ErrInternalServerError
	}
	if err := h.store.IncrementUsageQuota(c.Context(), userID, fmt.Sprintf("%.6f", float64(file.Size)/math.Pow10(9)), 0, 0, 0); err != nil {
		return fiber.ErrInternalServerError
	}
	return c.Status(fiber.StatusCreated).JSON(document)
}

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
	items, err := h.store.Documents(c.Context(), userID, limit, offset)
	if err != nil {
		return fiber.ErrInternalServerError
	}
	return c.JSON(items)
}

func (h *Handler) Get(c *fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return fiber.ErrBadRequest
	}
	item, err := h.ownedDocument(c, id)
	if err != nil {
		return err
	}
	return c.JSON(item)
}

type updateInput struct {
	Category string          `json:"category"`
	Summary  string          `json:"summary"`
	Metadata json.RawMessage `json:"metadata"`
}

func (h *Handler) Update(c *fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return fiber.ErrBadRequest
	}
	var value updateInput
	if err := c.BodyParser(&value); err != nil {
		return fiber.ErrBadRequest
	}
	metadata := string(value.Metadata)
	if len(value.Metadata) == 0 {
		metadata = "{}"
	}
	if !json.Valid([]byte(metadata)) {
		return fiber.NewError(fiber.StatusBadRequest, "metadata must be valid JSON")
	}
	if _, err := h.ownedDocument(c, id); err != nil {
		return err
	}
	item, err := h.store.UpdateDocument(c.Context(), database.Document{ID: id, Category: value.Category, Summary: value.Summary, Metadata: metadata})
	if err == database.ErrDocumentNotFound {
		return fiber.ErrNotFound
	}
	if err != nil {
		return fiber.ErrInternalServerError
	}
	return c.JSON(item)
}

func (h *Handler) Delete(c *fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return fiber.ErrBadRequest
	}
	item, err := h.ownedDocument(c, id)
	if err != nil {
		return err
	}
	// Delete from R2 first (idempotent), then soft-delete from DB
	if err := h.r2.Delete(c.Context(), item.R2Key); err != nil {
		return fiber.ErrInternalServerError
	}
	if err := h.store.SoftDeleteDocument(c.Context(), id); err != nil {
		return fiber.ErrInternalServerError
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) Download(c *fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return fiber.ErrBadRequest
	}
	item, err := h.ownedDocument(c, id)
	if err != nil {
		return err
	}
	object, err := h.r2.Get(c.Context(), item.R2Key)
	if err != nil {
		return fiber.ErrNotFound
	}
	defer object.Close()
	c.Set(fiber.HeaderContentType, r2storage.SafeContentType(item.MimeType))
	c.Set(fiber.HeaderContentDisposition, fmt.Sprintf(`attachment; filename="%s"`, strings.ReplaceAll(item.FileName, `"`, "")))
	c.Set(fiber.HeaderXContentTypeOptions, "nosniff")
	return c.SendStream(object)
}

func (h *Handler) Versions(c *fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return fiber.ErrBadRequest
	}
	if _, err := h.ownedDocument(c, id); err != nil {
		return err
	}
	items, err := h.store.DocumentVersions(c.Context(), id)
	if err != nil {
		return fiber.ErrInternalServerError
	}
	return c.JSON(items)
}

func (h *Handler) CreateTag(c *fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return fiber.ErrBadRequest
	}
	var value struct {
		Tag             string  `json:"tag"`
		ConfidenceScore *string `json:"confidence_score"`
	}
	if err := c.BodyParser(&value); err != nil || value.Tag == "" {
		return fiber.NewError(fiber.StatusBadRequest, "tag is required")
	}
	if _, err := h.ownedDocument(c, id); err != nil {
		return err
	}
	item, err := h.store.CreateDocumentTag(c.Context(), database.DocumentTag{DocumentID: id, Tag: value.Tag, ConfidenceScore: value.ConfidenceScore})
	if err != nil {
		return fiber.ErrConflict
	}
	return c.Status(fiber.StatusCreated).JSON(item)
}

func (h *Handler) ListTags(c *fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return fiber.ErrBadRequest
	}
	if _, err := h.ownedDocument(c, id); err != nil {
		return err
	}
	items, err := h.store.DocumentTags(c.Context(), id)
	if err != nil {
		return fiber.ErrInternalServerError
	}
	return c.JSON(items)
}

func (h *Handler) DeleteTag(c *fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return fiber.ErrBadRequest
	}
	tagID, err := strconv.ParseUint(c.Params("tagID"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}
	if _, err := h.ownedDocument(c, id); err != nil {
		return err
	}
	if err := h.store.DeleteDocumentTag(c.Context(), id, tagID); err == database.ErrDocumentTagNotFound {
		return fiber.ErrNotFound
	} else if err != nil {
		return fiber.ErrInternalServerError
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) ownedDocument(c *fiber.Ctx, id uint64) (database.Document, error) {
	item, err := h.store.Document(c.Context(), id)
	if errors.Is(err, database.ErrDocumentNotFound) {
		return database.Document{}, fiber.ErrNotFound
	}
	if err != nil {
		return database.Document{}, fiber.ErrInternalServerError
	}
	if item.UserID != middleware.UserID(c) {
		return database.Document{}, fiber.ErrNotFound
	}
	return item, nil
}

func parseID(c *fiber.Ctx) (uint64, error) { return strconv.ParseUint(c.Params("id"), 10, 64) }
func sanitizeName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "/", "_")
	if name == "" {
		return "document"
	}
	return name
}
