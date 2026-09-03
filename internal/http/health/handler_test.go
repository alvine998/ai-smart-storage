package health

import (
	"context"
	"net/http/httptest"
	"testing"

	"ai-smart-storage/internal/database"
	"ai-smart-storage/internal/storage"

	"github.com/gofiber/fiber/v2"
)

// MockStore is a mock database store for testing
type MockStore struct{}

func (m *MockStore) Ping(ctx context.Context) error {
	return nil
}

// MockR2 is a mock R2 store for testing
type MockR2 struct{}

func (m *MockR2) Ping(ctx context.Context) error {
	return nil
}

func TestHealth(t *testing.T) {
	app := fiber.New()
	// For testing, we can use nil or implement a simple mock
	// Here we're just checking that the endpoint responds
	h := &Handler{
		store: &database.Store{},
		r2:    &storage.Store{},
	}
	h.Register(app)

	response, err := app.Test(httptest.NewRequest("GET", "/health", nil))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusServiceUnavailable && response.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d (expected 200 or 503)", response.StatusCode)
	}
}
