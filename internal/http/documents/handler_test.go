package documents

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestCreateRequiresFile(t *testing.T) {
	app := fiber.New()
	New(nil, nil).Register(app)

	response, err := app.Test(httptest.NewRequest("POST", "/v1/documents", nil))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d", response.StatusCode)
	}
}

func TestSanitizeNameRemovesPathComponents(t *testing.T) {
	if got := sanitizeName(`../private/report.pdf`); got != ".._private_report.pdf" {
		t.Fatalf("sanitized name = %q", got)
	}
	if got := sanitizeName(" "); got != "document" {
		t.Fatalf("empty name = %q", got)
	}
}
