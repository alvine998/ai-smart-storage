package invoices

import (
	"strconv"

	"ai-smart-storage/internal/database"
	"ai-smart-storage/internal/http/middleware"
	"github.com/gofiber/fiber/v2"
)

type Handler struct{ store *database.Store }

func New(store *database.Store) *Handler { return &Handler{store: store} }

func (h *Handler) Register(app fiber.Router) {
	app.Post("/v1/invoices", h.Create)
	app.Get("/v1/invoices", h.List)
	app.Get("/v1/invoices/:id", h.Get)
	app.Put("/v1/invoices/:id", h.Update)
	app.Delete("/v1/invoices/:id", h.Delete)
}

type input struct {
	UserID         uint64 `json:"user_id"`
	SubscriptionID uint64 `json:"subscription_id"`
	Amount         string `json:"amount"`
	Status         string `json:"status"`
	PaymentMethod  string `json:"payment_method"`
	PaidAt         string `json:"paid_at"`
}

func (h *Handler) Create(c *fiber.Ctx) error {
	var value input
	if err := c.BodyParser(&value); err != nil || value.UserID == 0 || value.SubscriptionID == 0 || value.Amount == "" || !validStatus(value.Status) {
		return fiber.NewError(fiber.StatusBadRequest, "user_id, subscription_id, amount, and a valid status are required")
	}
	if value.UserID != middleware.UserID(c) {
		return fiber.NewError(fiber.StatusForbidden, "user_id does not match the authenticated user")
	}
	item, err := h.store.CreateInvoice(c.Context(), database.Invoice{UserID: middleware.UserID(c), SubscriptionID: value.SubscriptionID, Amount: value.Amount, Status: value.Status, PaymentMethod: value.PaymentMethod, PaidAt: value.PaidAt})
	if err != nil {
		return fiber.ErrConflict
	}
	return c.Status(fiber.StatusCreated).JSON(item)
}

func (h *Handler) List(c *fiber.Ctx) error {
	return fiber.NewError(fiber.StatusForbidden, "listing all invoices requires admin access")
}

func (h *Handler) Get(c *fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return fiber.ErrBadRequest
	}
	item, err := h.ownedInvoice(c, id)
	if err != nil {
		return err
	}
	return c.JSON(item)
}

func (h *Handler) Update(c *fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return fiber.ErrBadRequest
	}
	var value input
	if err := c.BodyParser(&value); err != nil || value.UserID == 0 || value.SubscriptionID == 0 || value.Amount == "" || !validStatus(value.Status) {
		return fiber.NewError(fiber.StatusBadRequest, "user_id, subscription_id, amount, and a valid status are required")
	}
	if value.UserID != middleware.UserID(c) {
		return fiber.NewError(fiber.StatusForbidden, "user_id does not match the authenticated user")
	}
	if _, err := h.ownedInvoice(c, id); err != nil {
		return err
	}
	item, err := h.store.UpdateInvoice(c.Context(), database.Invoice{ID: id, UserID: value.UserID, SubscriptionID: value.SubscriptionID, Amount: value.Amount, Status: value.Status, PaymentMethod: value.PaymentMethod, PaidAt: value.PaidAt})
	if err == database.ErrInvoiceNotFound {
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
	if _, err := h.ownedInvoice(c, id); err != nil {
		return err
	}
	if err := h.store.DeleteInvoice(c.Context(), id); err == database.ErrInvoiceNotFound {
		return fiber.ErrNotFound
	} else if err != nil {
		return fiber.ErrConflict
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) ownedInvoice(c *fiber.Ctx, id uint64) (database.Invoice, error) {
	item, err := h.store.Invoice(c.Context(), id)
	if err == database.ErrInvoiceNotFound {
		return database.Invoice{}, fiber.ErrNotFound
	}
	if err != nil {
		return database.Invoice{}, fiber.ErrInternalServerError
	}
	if item.UserID != middleware.UserID(c) {
		return database.Invoice{}, fiber.ErrNotFound
	}
	return item, nil
}

func validStatus(status string) bool {
	return status == "paid" || status == "pending" || status == "failed"
}
func parseID(c *fiber.Ctx) (uint64, error) { return strconv.ParseUint(c.Params("id"), 10, 64) }
