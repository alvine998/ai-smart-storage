package middleware

import (
	"strconv"
	"strings"

	"ai-smart-storage/internal/auth"

	"github.com/gofiber/fiber/v2"
)

func RequireAuth(secret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token, ok := strings.CutPrefix(c.Get(fiber.HeaderAuthorization), "Bearer ")
		if !ok || token == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "bearer token required")
		}
		userID, err := auth.Verify(secret, token)
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid or expired token")
		}
		c.Locals("user_id", userID)
		return c.Next()
	}
}

func UserID(c *fiber.Ctx) uint64 {
	id, _ := c.Locals("user_id").(uint64)
	return id
}

func SelfID(c *fiber.Ctx, requested string) (uint64, error) {
	userID := UserID(c)
	if requested == "" {
		if userID == 0 {
			return 0, fiber.NewError(fiber.StatusUnauthorized, "authentication required")
		}
		return userID, nil
	}
	parsed, err := strconv.ParseUint(requested, 10, 64)
	if err != nil || parsed != userID {
		return 0, fiber.NewError(fiber.StatusForbidden, "user_id does not match the authenticated user")
	}
	return userID, nil
}
