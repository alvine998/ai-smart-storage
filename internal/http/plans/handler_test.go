package plans

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestCreateRejectsUnknownPlanName(t *testing.T) {
	app := fiber.New()
	New(nil).Register(app)
	request := httptest.NewRequest("POST", "/v1/plans", strings.NewReader(`{"name":"Free","price":"0","storage_quota_gb":"1","is_active":true}`))
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d", response.StatusCode)
	}
}
