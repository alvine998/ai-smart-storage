package whatsapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestVerify(t *testing.T) {
	service := New("verify-me", "secret", "phone-id", "v22.0")
	challenge, err := service.Verify("subscribe", "challenge-value", "verify-me")
	if err != nil || challenge != "challenge-value" {
		t.Fatalf("challenge = %q, error = %v", challenge, err)
	}
	if _, err := service.Verify("subscribe", "challenge-value", "wrong-token"); err == nil {
		t.Fatal("expected invalid token error")
	}
}

func TestValidSignature(t *testing.T) {
	body := []byte(`{"object":"whatsapp_business_account"}`)
	mac := hmac.New(sha256.New, []byte("secret"))
	_, _ = mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	service := New("token", "secret", "phone-id", "v22.0")
	if !service.ValidSignature(body, signature) {
		t.Fatal("expected valid signature")
	}
	if service.ValidSignature(body, "sha256="+hex.EncodeToString([]byte("invalid"))) {
		t.Fatal("expected invalid signature")
	}
}
