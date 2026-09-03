package auth

import (
	"testing"
	"time"
)

func TestIssueAndVerifyRoundTrip(t *testing.T) {
	token, err := Issue("secret-key", 42, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	id, err := Verify("secret-key", token)
	if err != nil {
		t.Fatal(err)
	}
	if id != 42 {
		t.Fatalf("id = %d, want 42", id)
	}
}

func TestVerifyRejectsForeignSecret(t *testing.T) {
	token, err := Issue("secret-key", 42, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Verify("other-secret", token); err == nil {
		t.Fatal("expected error for token signed with a different secret")
	}
}

func TestVerifyRejectsExpiredToken(t *testing.T) {
	token, err := Issue("secret-key", 42, -time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Verify("secret-key", token); err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestVerifyRejectsGarbage(t *testing.T) {
	if _, err := Verify("secret-key", "not-a-token"); err == nil {
		t.Fatal("expected error for malformed token")
	}
}
