package users

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestCreateRejectsWeakPassword(t *testing.T) {
	app := fiber.New()
	New(nil).Register(app)

	request := httptest.NewRequest("POST", "/v1/users", strings.NewReader(`{"name":"Test User","email":"test@example.com","password":"short"}`))
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d", response.StatusCode)
	}
}
