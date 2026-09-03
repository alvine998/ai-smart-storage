package whatsapp

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"testing"

	"ai-smart-storage/internal/whatsapp"

	"github.com/gofiber/fiber/v2"
)

func TestVerifySuccess(t *testing.T) {
	wa := whatsapp.New("verify-token", "secret", "phone-id", "v22.0")
	h := New(nil, nil, wa, "http://example.com/signup", nil)
	app := fiber.New()
	h.Register(app)

	req := httptest.NewRequest("GET", "/webhooks/whatsapp?hub.mode=subscribe&hub.challenge=CHALLENGE&hub.verify_token=verify-token", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	if buf.String() != "CHALLENGE" {
		t.Fatalf("body = %q, want CHALLENGE", buf.String())
	}
}

func TestVerifyRejectsBadToken(t *testing.T) {
	wa := whatsapp.New("correct-token", "secret", "phone-id", "v22.0")
	h := New(nil, nil, wa, "http://example.com/signup", nil)
	app := fiber.New()
	h.Register(app)

	req := httptest.NewRequest("GET", "/webhooks/whatsapp?hub.mode=subscribe&hub.challenge=CHALLENGE&hub.verify_token=wrong", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusForbidden)
	}
}

func TestReceiveRejectsInvalidSignature(t *testing.T) {
	wa := whatsapp.New("token", "my-secret", "phone-id", "v22.0")
	h := New(nil, nil, wa, "http://example.com/signup", nil)
	app := fiber.New()
	h.Register(app)

	body := []byte(`{"entry":[{"changes":[{"value":{"messages":[{"type":"text","id":"wamid.123","from":"15551234567","text":{"body":"hello"}}]}}]}]}`)
	req := httptest.NewRequest("POST", "/webhooks/whatsapp", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusUnauthorized)
	}
}

func TestReceiveAcceptsValidSignature(t *testing.T) {
	secret := "my-secret"
	wa := whatsapp.New("token", secret, "phone-id", "v22.0")
	h := New(nil, nil, wa, "http://example.com/signup", nil)
	app := fiber.New()
	h.Register(app)

	body := []byte(`{"entry":[{"changes":[{"value":{"messages":[{"type":"text","id":"wamid.123","from":"15551234567","text":{"body":"hello"}}]}}]}]}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("POST", "/webhooks/whatsapp", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sig)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}
}

func TestReceiveRejectsInvalidJSON(t *testing.T) {
	secret := "secret"
	wa := whatsapp.New("token", secret, "phone-id", "v22.0")
	h := New(nil, nil, wa, "http://example.com/signup", nil)
	app := fiber.New()
	h.Register(app)

	body := []byte(`{invalid json}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("POST", "/webhooks/whatsapp", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sig)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusBadRequest)
	}
}

func TestReceiveIgnoresNonTextMessages(t *testing.T) {
	secret := "secret"
	wa := whatsapp.New("token", secret, "phone-id", "v22.0")
	h := New(nil, nil, wa, "http://example.com/signup", nil)
	app := fiber.New()
	h.Register(app)

	body := []byte(`{"entry":[{"changes":[{"value":{"messages":[{"type":"image","id":"wamid.123","from":"15551234567"}]}}]}]}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("POST", "/webhooks/whatsapp", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sig)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestListConversationsRequiresAuth(t *testing.T) {
	wa := whatsapp.New("token", "secret", "phone-id", "v22.0")
	h := New(nil, nil, wa, "http://example.com/signup", nil)
	app := fiber.New()
	// Register protected route without auth middleware - but handler calls middleware.SelfID which checks user_id
	h.RegisterProtected(app)

	req := httptest.NewRequest("GET", "/v1/wa-conversations?user_id=1", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	// Without user_id in locals, SelfID should return 401 Unauthorized
	if resp.StatusCode != fiber.StatusUnauthorized && resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("status = %d, want 401 or 403", resp.StatusCode)
	}
}

func TestListConversationsWithAuth(t *testing.T) {
	wa := whatsapp.New("token", "secret", "phone-id", "v22.0")
	h := New(nil, nil, wa, "http://example.com/signup", nil)
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("user_id", uint64(1))
		return c.Next()
	})
	h.RegisterProtected(app)

	// Store is nil, so WAConversations will panic? Handler calls h.store.WAConversations
	// When store is nil, it will panic nil dereference -> 500.
	// We expect 500 because store not mocked.
	req := httptest.NewRequest("GET", "/v1/wa-conversations?user_id=1", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	// Should be internal server error due to nil store
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
}
