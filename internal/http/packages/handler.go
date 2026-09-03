package packages

import (
	"strconv"

	"ai-smart-storage/internal/database"
	"github.com/gofiber/fiber/v2"
)

type Handler struct{ store *database.Store }

func New(store *database.Store) *Handler { return &Handler{store: store} }

func (h *Handler) Register(app fiber.Router) {
	app.Post("/v1/packages", h.Create)
	app.Get("/v1/packages", h.List)
	app.Get("/v1/packages/:id", h.Get)
	app.Put("/v1/packages/:id", h.Update)
	app.Delete("/v1/packages/:id", h.Delete)
}

type input struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Price       string `json:"price"`
}

func (h *Handler) Create(c *fiber.Ctx) error {
	var value input
	if err := c.BodyParser(&value); err != nil || value.Name == "" || value.Price == "" {
		return fiber.NewError(fiber.StatusBadRequest, "name and price are required")
	}
	item, err := h.store.CreatePackage(c.Context(), database.Package{Name: value.Name, Description: value.Description, Price: value.Price})
	if err != nil {
		return fiber.ErrConflict
	}
	return c.Status(fiber.StatusCreated).JSON(item)
}

func (h *Handler) List(c *fiber.Ctx) error {
	items, err := h.store.Packages(c.Context())
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
	item, err := h.store.Package(c.Context(), id)
	if err == database.ErrPackageNotFound {
		return fiber.ErrNotFound
	}
	if err != nil {
		return fiber.ErrInternalServerError
	}
	return c.JSON(item)
}

func (h *Handler) Update(c *fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return fiber.ErrBadRequest
	}
	var value input
	if err := c.BodyParser(&value); err != nil || value.Name == "" || value.Price == "" {
		return fiber.NewError(fiber.StatusBadRequest, "name and price are required")
	}
	item, err := h.store.UpdatePackage(c.Context(), database.Package{ID: id, Name: value.Name, Description: value.Description, Price: value.Price})
	if err == database.ErrPackageNotFound {
		return fiber.ErrNotFound
	}
	if err != nil {
		return fiber.ErrConflict
	}
	return c.JSON(item)
}

func (h *Handler) Delete(c *fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return fiber.ErrBadRequest
	}
	if err := h.store.DeletePackage(c.Context(), id); err == database.ErrPackageNotFound {
		return fiber.ErrNotFound
	} else if err != nil {
		return fiber.ErrInternalServerError
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func parseID(c *fiber.Ctx) (uint64, error) { return strconv.ParseUint(c.Params("id"), 10, 64) }
