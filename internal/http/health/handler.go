package health

import "github.com/gofiber/fiber/v2"

type Handler struct{}

func New() *Handler { return &Handler{} }

func (h *Handler) Register(app fiber.Router) {
	app.Get("/health", h.Health)
}

func (h *Handler) Health(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "ok"})
}
