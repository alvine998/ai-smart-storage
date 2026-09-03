package users

import (
	"strconv"

	"ai-smart-storage/internal/database"
	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"
)

type Handler struct{ store *database.Store }

func New(store *database.Store) *Handler { return &Handler{store: store} }

func (h *Handler) Register(app fiber.Router) {
	app.Post("/v1/users", h.Create)
	app.Get("/v1/users", h.List)
	app.Get("/v1/users/:id", h.Get)
	app.Put("/v1/users/:id", h.Update)
	app.Delete("/v1/users/:id", h.Delete)
}

type input struct {
	Name        string `json:"name"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	PhoneNumber string `json:"phone_number"`
}

func (h *Handler) Create(c *fiber.Ctx) error {
	var value input
	if err := c.BodyParser(&value); err != nil || value.Name == "" || value.Email == "" || len(value.Password) < 8 {
		return fiber.NewError(fiber.StatusBadRequest, "name, email, and a password of at least 8 characters are required")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(value.Password), bcrypt.DefaultCost)
	if err != nil {
		return fiber.ErrInternalServerError
	}
	user, err := h.store.CreateUser(c.Context(), database.User{Name: value.Name, Email: value.Email, PasswordHash: string(hash), PhoneNumber: value.PhoneNumber})
	if err != nil {
		return fiber.ErrConflict
	}
	return c.Status(fiber.StatusCreated).JSON(user)
}

func (h *Handler) List(c *fiber.Ctx) error {
	users, err := h.store.Users(c.Context())
	if err != nil {
		return fiber.ErrInternalServerError
	}
	return c.JSON(users)
}

func (h *Handler) Get(c *fiber.Ctx) error {
	id, err := idParam(c)
	if err != nil {
		return fiber.ErrBadRequest
	}
	user, err := h.store.User(c.Context(), id)
	if err == database.ErrUserNotFound {
		return fiber.ErrNotFound
	}
	if err != nil {
		return fiber.ErrInternalServerError
	}
	return c.JSON(user)
}

func (h *Handler) Update(c *fiber.Ctx) error {
	id, err := idParam(c)
	if err != nil {
		return fiber.ErrBadRequest
	}
	var value input
	if err := c.BodyParser(&value); err != nil || value.Name == "" || value.Email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "name and email are required")
	}
	if value.Password != "" && len(value.Password) < 8 {
		return fiber.NewError(fiber.StatusBadRequest, "password must be at least 8 characters")
	}
	user, err := h.store.UpdateUser(c.Context(), database.User{ID: id, Name: value.Name, Email: value.Email, PhoneNumber: value.PhoneNumber})
	if err == database.ErrUserNotFound {
		return fiber.ErrNotFound
	}
	if err != nil {
		return fiber.ErrConflict
	}
	if value.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(value.Password), bcrypt.DefaultCost)
		if err != nil {
			return fiber.ErrInternalServerError
		}
		if err := h.store.UpdatePassword(c.Context(), id, string(hash)); err != nil {
			return fiber.ErrInternalServerError
		}
	}
	return c.JSON(user)
}

func (h *Handler) Delete(c *fiber.Ctx) error {
	id, err := idParam(c)
	if err != nil {
		return fiber.ErrBadRequest
	}
	if err := h.store.DeleteUser(c.Context(), id); err == database.ErrUserNotFound {
		return fiber.ErrNotFound
	} else if err != nil {
		return fiber.ErrInternalServerError
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func idParam(c *fiber.Ctx) (uint64, error) { return strconv.ParseUint(c.Params("id"), 10, 64) }
