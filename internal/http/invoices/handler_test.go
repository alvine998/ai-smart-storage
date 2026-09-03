package invoices

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestCreateRejectsUnknownStatus(t *testing.T) {
	app := fiber.New()
	New(nil).Register(app)
	request := httptest.NewRequest("POST", "/v1/invoices", strings.NewReader(`{"user_id":1,"subscription_id":1,"amount":"10.00","status":"open"}`))
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d", response.StatusCode)
	}
}
