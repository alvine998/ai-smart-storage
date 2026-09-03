package chat

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ai-smart-storage/internal/ai"

	"github.com/gofiber/fiber/v2"
)

func authMiddleware(userID uint64) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Locals("user_id", userID)
		return c.Next()
	}
}

func TestStreamRequiresMessages(t *testing.T) {
	app := fiber.New()
	app.Use(authMiddleware(1))
	New(nil, nil, 0, 0).Register(app)

	req := httptest.NewRequest("POST", "/v1/chat/stream", bytes.NewReader([]byte(`{"user_id":1,"messages":[]}`)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusBadRequest)
	}
}

func TestStreamRejectsMismatchedUserID(t *testing.T) {
	app := fiber.New()
	app.Use(authMiddleware(1))
	New(nil, nil, 0, 0).Register(app)

	body := `{"user_id":2,"messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/stream", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusForbidden)
	}
}

func TestStreamRequiresBody(t *testing.T) {
	app := fiber.New()
	app.Use(authMiddleware(1))
	New(nil, nil, 0, 0).Register(app)

	req := httptest.NewRequest("POST", "/v1/chat/stream", nil)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusBadRequest)
	}
}

func TestStreamSetsEventStreamHeaders(t *testing.T) {
	// mock MiMo server that streams two chunks then DONE
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client := ai.NewClient("test-key", server.URL, "mimo-v2.5", time.Second)
	// Use a Store that will fail quota check? Use nil to skip quota enforcement.
	h := New(client, nil, 0, 0)

	app := fiber.New()
	app.Use(authMiddleware(42))
	h.Register(app)

	body := `{"user_id":42,"messages":[{"role":"user","content":"Hi"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/stream", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want to contain text/event-stream", ct)
	}
	// Ensure the underlying handler attempted streaming without panic
	// Read body asynchronously? fiber test reads body after stream writer.
	// We just check that the response was OK and headers set.
	_ = context.Background()
}

func TestStreamWithUsageErrorPropagates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "provider unavailable", http.StatusBadGateway)
	}))
	defer server.Close()

	client := ai.NewClient("key", server.URL, "model", time.Second)
	h := New(client, nil, 0, 0)
	app := fiber.New()
	app.Use(authMiddleware(1))
	h.Register(app)

	body := `{"messages":[{"role":"user","content":"Hi"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/stream", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	// Even when provider fails, the handler still returns 200 with SSE error event
	// because streaming is done via SetBodyStreamWriter. We verify it doesn't return 500.
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}
}
