package usagequota

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestGetRequiresUserID(t *testing.T) {
	app := fiber.New()
	New(nil).Register(app)
	response, err := app.Test(httptest.NewRequest("GET", "/v1/usage-quota", nil))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d", response.StatusCode)
	}
}
