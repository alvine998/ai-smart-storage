package ai_logs

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestListRequiresUserID(t *testing.T) {
	app := fiber.New()
	New(nil).Register(app)
	response, err := app.Test(httptest.NewRequest("GET", "/v1/ai-processing-logs", nil))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d", response.StatusCode)
	}
}
