package subscriptions

import (
	"strconv"

	"ai-smart-storage/internal/database"
	"github.com/gofiber/fiber/v2"
)

type Handler struct{ store *database.Store }

func New(store *database.Store) *Handler { return &Handler{store: store} }

func (h *Handler) Register(app fiber.Router) {
	app.Post("/v1/subscriptions", h.Create)
	app.Get("/v1/subscriptions", h.List)
	app.Get("/v1/subscriptions/:id", h.Get)
	app.Put("/v1/subscriptions/:id", h.Update)
	app.Delete("/v1/subscriptions/:id", h.Delete)
}

type input struct {
	UserID             uint64 `json:"user_id"`
	PlanID             uint64 `json:"plan_id"`
	Status             string `json:"status"`
	CurrentPeriodStart string `json:"current_period_start"`
	CurrentPeriodEnd   string `json:"current_period_end"`
}

func (h *Handler) Create(c *fiber.Ctx) error {
	var value input
	if err := c.BodyParser(&value); err != nil || !validStatus(value.Status) || value.UserID == 0 || value.PlanID == 0 || value.CurrentPeriodStart == "" || value.CurrentPeriodEnd == "" {
		return fiber.NewError(fiber.StatusBadRequest, "user_id, plan_id, status, current_period_start, and current_period_end are required")
	}
	item, err := h.store.CreateSubscription(c.Context(), database.Subscription{UserID: value.UserID, PlanID: value.PlanID, Status: value.Status, CurrentPeriodStart: value.CurrentPeriodStart, CurrentPeriodEnd: value.CurrentPeriodEnd})
	if err != nil {
		return fiber.ErrConflict
	}
	return c.Status(fiber.StatusCreated).JSON(item)
}

func (h *Handler) List(c *fiber.Ctx) error {
	items, err := h.store.Subscriptions(c.Context())
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
	item, err := h.store.Subscription(c.Context(), id)
	if err == database.ErrSubscriptionNotFound {
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
	if err := c.BodyParser(&value); err != nil || !validStatus(value.Status) || value.UserID == 0 || value.PlanID == 0 || value.CurrentPeriodStart == "" || value.CurrentPeriodEnd == "" {
		return fiber.NewError(fiber.StatusBadRequest, "user_id, plan_id, status, current_period_start, and current_period_end are required")
	}
	item, err := h.store.UpdateSubscription(c.Context(), database.Subscription{ID: id, UserID: value.UserID, PlanID: value.PlanID, Status: value.Status, CurrentPeriodStart: value.CurrentPeriodStart, CurrentPeriodEnd: value.CurrentPeriodEnd})
	if err == database.ErrSubscriptionNotFound {
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
	if err := h.store.DeleteSubscription(c.Context(), id); err == database.ErrSubscriptionNotFound {
		return fiber.ErrNotFound
	} else if err != nil {
		return fiber.ErrConflict
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func validStatus(status string) bool {
	return status == "active" || status == "past_due" || status == "canceled"
}
func parseID(c *fiber.Ctx) (uint64, error) { return strconv.ParseUint(c.Params("id"), 10, 64) }
