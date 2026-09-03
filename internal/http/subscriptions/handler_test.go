package subscriptions

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestCreateRejectsUnknownStatus(t *testing.T) {
	app := fiber.New()
	New(nil).Register(app)
	request := httptest.NewRequest("POST", "/v1/subscriptions", strings.NewReader(`{"user_id":1,"plan_id":1,"status":"expired","current_period_start":"2026-09-01 00:00:00","current_period_end":"2026-10-01 00:00:00"}`))
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d", response.StatusCode)
	}
}
