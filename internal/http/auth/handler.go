package auth

import (
	"errors"
	"time"

	"ai-smart-storage/internal/auth"
	"ai-smart-storage/internal/database"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"golang.org/x/crypto/bcrypt"
)

type Handler struct {
	store  *database.Store
	secret string
	ttl    time.Duration
}

func New(store *database.Store, secret string, ttl time.Duration) *Handler {
	return &Handler{store: store, secret: secret, ttl: ttl}
}

func (h *Handler) Register(app fiber.Router) {
	app.Post("/v1/auth/login", limiter.New(limiter.Config{Max: 10, Expiration: time.Minute}), h.Login)
}

type loginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) Login(c *fiber.Ctx) error {
	var value loginInput
	if err := c.BodyParser(&value); err != nil || value.Email == "" || value.Password == "" {
		return fiber.ErrBadRequest
	}
	user, err := h.store.UserByEmail(c.Context(), value.Email)
	if errors.Is(err, database.ErrUserNotFound) {
		return fiber.ErrUnauthorized
	}
	if err != nil {
		return fiber.ErrInternalServerError
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(value.Password)) != nil {
		return fiber.ErrUnauthorized
	}
	token, err := auth.Issue(h.secret, user.ID, h.ttl)
	if err != nil {
		return fiber.ErrInternalServerError
	}
	return c.JSON(fiber.Map{"token": token, "token_type": "Bearer", "expires_in": int(h.ttl.Seconds())})
}
