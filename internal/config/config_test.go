package config

import "testing"

func TestLoadUsesDefaults(t *testing.T) {
	keys := []string{"APP_PORT", "MIMO_BASE_URL", "MIMO_MODEL", "MIMO_TIMEOUT_SECONDS", "R2_BUCKET", "WHATSAPP_GRAPH_VERSION"}
	for _, key := range keys {
		t.Setenv(key, "")
	}

	got := Load()
	if got.Port != "8080" || got.MimoModel != "mimo-v2.5" || got.R2Bucket != "ai-smart-storage" {
		t.Fatalf("unexpected defaults: %+v", got)
	}
	if got.MimoTimeoutSec != 120 || got.WhatsAppGraphVer != "v22.0" {
		t.Fatalf("unexpected numeric/version defaults: %+v", got)
	}
}

func TestLoadReadsConfiguredValues(t *testing.T) {
	t.Setenv("APP_PORT", "9090")
	t.Setenv("MIMO_TIMEOUT_SECONDS", "30")
	t.Setenv("R2_BUCKET", "documents")

	got := Load()
	if got.Port != "9090" || got.MimoTimeoutSec != 30 || got.R2Bucket != "documents" {
		t.Fatalf("configured values not loaded: %+v", got)
	}
}
