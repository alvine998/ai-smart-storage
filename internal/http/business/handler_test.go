package business

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestCreateRequiresLegalName(t *testing.T) {
	app := fiber.New()
	New(nil).Register(app)

	request := httptest.NewRequest("POST", "/v1/users/1/business", strings.NewReader(`{"display_name":"Optional Business"}`))
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d", response.StatusCode)
	}
}
