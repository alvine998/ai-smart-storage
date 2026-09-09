package whatsapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestVerify(t *testing.T) {
	service := New("token", "verify-me", "secret", "phone-id", "v22.0")
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

	service := New("token", "verify-token", "secret", "phone-id", "v22.0")
	if !service.ValidSignature(body, signature) {
		t.Fatal("expected valid signature")
	}
	if service.ValidSignature(body, "sha256="+hex.EncodeToString([]byte("invalid"))) {
		t.Fatal("expected invalid signature")
	}
}

func TestIsConfiguredRequiresWebhookCredentials(t *testing.T) {
	base := []string{"access-token", "verify-token", "app-secret", "phone-id", "v22.0"}
	for i, name := range []string{"access token", "verify token", "app secret", "phone ID", "graph version"} {
		values := append([]string(nil), base...)
		values[i] = ""
		service := New(values[0], values[1], values[2], values[3], values[4])
		if service.IsConfigured() {
			t.Errorf("IsConfigured() = true with missing %s", name)
		}
	}
}

func TestValidSignatureRequiresAppSecret(t *testing.T) {
	service := New("access-token", "verify-token", "", "phone-id", "v22.0")
	if service.ValidSignature([]byte("body"), "sha256=anything") {
		t.Fatal("ValidSignature() = true with empty app secret")
	}
}

func TestVerifyRequiresVerifyToken(t *testing.T) {
	service := New("access-token", "", "app-secret", "phone-id", "v22.0")
	if _, err := service.Verify("subscribe", "challenge", ""); err == nil {
		t.Fatal("Verify() succeeded with empty verify token")
	}
}
