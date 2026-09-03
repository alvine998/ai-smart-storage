package ai_logs

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestListRequiresAuth(t *testing.T) {
	app := fiber.New()
	New(nil).Register(app)
	response, err := app.Test(httptest.NewRequest("GET", "/v1/ai-processing-logs", nil))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d", response.StatusCode)
	}
}
