package business

import (
	"strconv"

	"ai-smart-storage/internal/database"
	"github.com/gofiber/fiber/v2"
)

type Handler struct{ store *database.Store }

func New(store *database.Store) *Handler { return &Handler{store: store} }

func (h *Handler) Register(app fiber.Router) {
	app.Post("/v1/users/:id/business", h.Create)
	app.Get("/v1/users/:id/business", h.Get)
	app.Put("/v1/users/:id/business", h.Update)
	app.Delete("/v1/users/:id/business", h.Delete)
}

type input struct {
	LegalName   string `json:"legal_name"`
	DisplayName string `json:"display_name"`
	TaxID       string `json:"tax_id"`
	PhoneNumber string `json:"phone_number"`
	Email       string `json:"email"`
	Website     string `json:"website"`
	Address     string `json:"address"`
}

func (h *Handler) Create(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return fiber.ErrBadRequest
	}
	var value input
	if err := c.BodyParser(&value); err != nil || value.LegalName == "" {
		return fiber.NewError(fiber.StatusBadRequest, "legal_name is required")
	}
	business, err := h.store.CreateBusiness(c.Context(), toBusiness(userID, 0, value))
	if err != nil {
		return fiber.ErrConflict
	}
	return c.Status(fiber.StatusCreated).JSON(business)
}

func (h *Handler) Get(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return fiber.ErrBadRequest
	}
	business, err := h.store.UserBusiness(c.Context(), userID)
	if err == database.ErrBusinessNotFound {
		return fiber.ErrNotFound
	}
	if err != nil {
		return fiber.ErrInternalServerError
	}
	return c.JSON(business)
}

func (h *Handler) Update(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return fiber.ErrBadRequest
	}
	business, err := h.store.UserBusiness(c.Context(), userID)
	if err == database.ErrBusinessNotFound {
		return fiber.ErrNotFound
	}
	if err != nil {
		return fiber.ErrInternalServerError
	}
	var value input
	if err := c.BodyParser(&value); err != nil || value.LegalName == "" {
		return fiber.NewError(fiber.StatusBadRequest, "legal_name is required")
	}
	updated, err := h.store.UpdateBusiness(c.Context(), toBusiness(userID, business.ID, value))
	if err == database.ErrBusinessNotFound {
		return fiber.ErrNotFound
	}
	if err != nil {
		return fiber.ErrInternalServerError
	}
	return c.JSON(updated)
}

func (h *Handler) Delete(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return fiber.ErrBadRequest
	}
	business, err := h.store.UserBusiness(c.Context(), userID)
	if err == database.ErrBusinessNotFound {
		return fiber.ErrNotFound
	}
	if err != nil {
		return fiber.ErrInternalServerError
	}
	if err := h.store.DeleteBusiness(c.Context(), userID, business.ID); err != nil {
		return fiber.ErrInternalServerError
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func parseUserID(c *fiber.Ctx) (uint64, error) { return strconv.ParseUint(c.Params("id"), 10, 64) }

func toBusiness(userID, id uint64, value input) database.Business {
	return database.Business{ID: id, UserID: userID, LegalName: value.LegalName, DisplayName: value.DisplayName, TaxID: value.TaxID, PhoneNumber: value.PhoneNumber, Email: value.Email, Website: value.Website, Address: value.Address}
}
