package plans

import (
	"strconv"

	"ai-smart-storage/internal/database"
	"github.com/gofiber/fiber/v2"
)

type Handler struct{ store *database.Store }

func New(store *database.Store) *Handler { return &Handler{store: store} }

func (h *Handler) Register(app fiber.Router) {
	app.Post("/v1/plans", h.Create)
	app.Get("/v1/plans", h.List)
	app.Get("/v1/plans/:id", h.Get)
	app.Put("/v1/plans/:id", h.Update)
	app.Delete("/v1/plans/:id", h.Delete)
}

type input struct {
	Name           string `json:"name"`
	Price          string `json:"price"`
	StorageQuotaGB string `json:"storage_quota_gb"`
	AIDocsQuota    uint64 `json:"ai_docs_quota"`
	AIQueryQuota   uint64 `json:"ai_query_quota"`
	WAMessageQuota uint64 `json:"wa_message_quota"`
	IsActive       *bool  `json:"is_active"`
}

func (h *Handler) Create(c *fiber.Ctx) error {
	var value input
	if err := c.BodyParser(&value); err != nil || !validName(value.Name) || value.Price == "" || value.StorageQuotaGB == "" || value.IsActive == nil {
		return fiber.NewError(fiber.StatusBadRequest, "name, price, storage_quota_gb, and is_active are required")
	}
	plan, err := h.store.CreatePlan(c.Context(), toPlan(0, value))
	if err != nil {
		return fiber.ErrConflict
	}
	return c.Status(fiber.StatusCreated).JSON(plan)
}

func (h *Handler) List(c *fiber.Ctx) error {
	items, err := h.store.Plans(c.Context())
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
	plan, err := h.store.Plan(c.Context(), id)
	if err == database.ErrPlanNotFound {
		return fiber.ErrNotFound
	}
	if err != nil {
		return fiber.ErrInternalServerError
	}
	return c.JSON(plan)
}

func (h *Handler) Update(c *fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return fiber.ErrBadRequest
	}
	var value input
	if err := c.BodyParser(&value); err != nil || !validName(value.Name) || value.Price == "" || value.StorageQuotaGB == "" || value.IsActive == nil {
		return fiber.NewError(fiber.StatusBadRequest, "name, price, storage_quota_gb, and is_active are required")
	}
	plan, err := h.store.UpdatePlan(c.Context(), toPlan(id, value))
	if err == database.ErrPlanNotFound {
		return fiber.ErrNotFound
	}
	if err != nil {
		return fiber.ErrConflict
	}
	return c.JSON(plan)
}

func (h *Handler) Delete(c *fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return fiber.ErrBadRequest
	}
	if err := h.store.DeletePlan(c.Context(), id); err == database.ErrPlanNotFound {
		return fiber.ErrNotFound
	} else if err != nil {
		return fiber.ErrConflict
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func validName(name string) bool {
	return name == "Starter" || name == "Business" || name == "Enterprise"
}
func parseID(c *fiber.Ctx) (uint64, error) { return strconv.ParseUint(c.Params("id"), 10, 64) }
func toPlan(id uint64, value input) database.Plan {
	return database.Plan{ID: id, Name: value.Name, Price: value.Price, StorageQuotaGB: value.StorageQuotaGB, AIDocsQuota: value.AIDocsQuota, AIQueryQuota: value.AIQueryQuota, WAMessageQuota: value.WAMessageQuota, IsActive: *value.IsActive}
}
