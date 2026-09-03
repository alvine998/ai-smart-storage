package database

import (
	"testing"
	"time"
)

func TestWhatsAppAccessWithinQuota(t *testing.T) {
	access := WhatsAppAccess{
		StorageLimitGB: "10",
		StorageUsedGB:  "2.5",
		AIDocsLimit:    10,
		AIDocsUsed:     9,
		AIQueryLimit:   20,
		AIQueriesUsed:  19,
		WALimit:        50,
		WAUsed:         49,
	}
	if !access.WithinQuota() {
		t.Fatal("expected access while every quota is below its limit")
	}
	access.WAUsed = access.WALimit
	if access.WithinQuota() {
		t.Fatal("expected access to stop at the WhatsApp message limit")
	}
}

func TestWhatsAppAccessInGracePeriod(t *testing.T) {
	access := WhatsAppAccess{PeriodEnd: time.Now().Add(-time.Hour)}
	if !access.InGracePeriod(time.Now()) {
		t.Fatal("expected expired subscription to be in grace period")
	}
}
