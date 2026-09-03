package storage

import "testing"

func TestNewRequiresCredentials(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("expected missing credentials error")
	}
}

func TestPublicURL(t *testing.T) {
	store := &Store{publicURL: "https://files.example.com"}
	if got := store.PublicURL("folder/report.pdf"); got != "https://files.example.com/folder/report.pdf" {
		t.Fatalf("public URL = %q", got)
	}
}
