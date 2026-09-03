package users

import (
	"strconv"

	"ai-smart-storage/internal/database"
	"ai-smart-storage/internal/http/middleware"
	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"
)

type Handler struct{ store *database.Store }

func New(store *database.Store) *Handler { return &Handler{store: store} }

func (h *Handler) Register(app fiber.Router) {
	app.Get("/v1/users", h.List)
	app.Get("/v1/users/:id", h.Get)
	app.Put("/v1/users/:id", h.Update)
	app.Delete("/v1/users/:id", h.Delete)
}

func (h *Handler) RegisterPublic(app fiber.Router) {
	app.Post("/v1/users", h.Create)
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
	return fiber.NewError(fiber.StatusForbidden, "listing all users requires admin access")
}

func (h *Handler) Get(c *fiber.Ctx) error {
	id, err := idParam(c)
	if err != nil {
		return fiber.ErrBadRequest
	}
	if id != middleware.UserID(c) {
		return fiber.NewError(fiber.StatusForbidden, "you can only access your own user record")
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
	if id != middleware.UserID(c) {
		return fiber.NewError(fiber.StatusForbidden, "you can only update your own user record")
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
	if id != middleware.UserID(c) {
		return fiber.NewError(fiber.StatusForbidden, "you can only delete your own user record")
	}
	if err := h.store.DeleteUser(c.Context(), id); err == database.ErrUserNotFound {
		return fiber.ErrNotFound
	} else if err != nil {
		return fiber.ErrInternalServerError
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func idParam(c *fiber.Ctx) (uint64, error) { return strconv.ParseUint(c.Params("id"), 10, 64) }
