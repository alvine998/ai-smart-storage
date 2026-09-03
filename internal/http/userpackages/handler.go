package userpackages

import (
	"strconv"

	"ai-smart-storage/internal/database"
	"github.com/gofiber/fiber/v2"
)

type Handler struct{ store *database.Store }

func New(store *database.Store) *Handler { return &Handler{store: store} }

func (h *Handler) Register(app fiber.Router) {
	app.Post("/v1/users/:id/packages", h.Create)
	app.Get("/v1/users/:id/packages", h.List)
	app.Put("/v1/users/:id/packages/:assignmentID", h.Update)
	app.Delete("/v1/users/:id/packages/:assignmentID", h.Delete)
}

type input struct {
	PackageID uint64 `json:"package_id"`
	Status    string `json:"status"`
	ExpiresAt string `json:"expires_at"`
}

func (h *Handler) Create(c *fiber.Ctx) error {
	userID, err := userID(c)
	if err != nil {
		return fiber.ErrBadRequest
	}
	var value input
	if err := c.BodyParser(&value); err != nil || value.PackageID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "package_id is required")
	}
	if value.Status == "" {
		value.Status = "active"
	}
	if !validStatus(value.Status) {
		return fiber.NewError(fiber.StatusBadRequest, "status must be active, paused, or cancelled")
	}
	item, err := h.store.CreateUserPackage(c.Context(), database.UserPackage{UserID: userID, PackageID: value.PackageID, Status: value.Status, ExpiresAt: value.ExpiresAt})
	if err != nil {
		return fiber.ErrConflict
	}
	return c.Status(fiber.StatusCreated).JSON(item)
}

func (h *Handler) List(c *fiber.Ctx) error {
	id, err := userID(c)
	if err != nil {
		return fiber.ErrBadRequest
	}
	items, err := h.store.UserPackages(c.Context(), id)
	if err != nil {
		return fiber.ErrInternalServerError
	}
	return c.JSON(items)
}

func (h *Handler) Update(c *fiber.Ctx) error {
	uid, err := userID(c)
	if err != nil {
		return fiber.ErrBadRequest
	}
	id, err := strconv.ParseUint(c.Params("assignmentID"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}
	var value input
	if err := c.BodyParser(&value); err != nil || value.PackageID == 0 || !validStatus(value.Status) {
		return fiber.NewError(fiber.StatusBadRequest, "package_id and a valid status are required")
	}
	item, err := h.store.UpdateUserPackage(c.Context(), database.UserPackage{ID: id, UserID: uid, PackageID: value.PackageID, Status: value.Status, ExpiresAt: value.ExpiresAt})
	if err == database.ErrUserPackageNotFound {
		return fiber.ErrNotFound
	}
	if err != nil {
		return fiber.ErrConflict
	}
	return c.JSON(item)
}

func (h *Handler) Delete(c *fiber.Ctx) error {
	uid, err := userID(c)
	if err != nil {
		return fiber.ErrBadRequest
	}
	id, err := strconv.ParseUint(c.Params("assignmentID"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}
	if err := h.store.DeleteUserPackage(c.Context(), uid, id); err == database.ErrUserPackageNotFound {
		return fiber.ErrNotFound
	} else if err != nil {
		return fiber.ErrInternalServerError
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func userID(c *fiber.Ctx) (uint64, error) { return strconv.ParseUint(c.Params("id"), 10, 64) }

func validStatus(status string) bool {
	return status == "active" || status == "paused" || status == "cancelled"
}
