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

func TestCanConsume(t *testing.T) {
	access := WhatsAppAccess{
		StorageLimitGB: "10",
		StorageUsedGB:  "9.5",
		AIDocsLimit:    10,
		AIDocsUsed:     5,
		AIQueryLimit:   20,
		AIQueriesUsed:  10,
		WALimit:        50,
		WAUsed:         10,
	}
	if !access.CanConsume(0.4, 0, 0, 0) {
		t.Fatal("expected to allow 0.4GB when 0.5GB remains")
	}
	if access.CanConsume(0.6, 0, 0, 0) {
		t.Fatal("expected storage exceed to be denied")
	}
	if !access.CanConsume(0, 1, 1, 1) {
		t.Fatal("expected small increments to be allowed")
	}
	access.AIQueriesUsed = access.AIQueryLimit - 1
	if access.CanConsume(0, 0, 1, 0) {
		t.Fatal("expected AI query at limit to be denied")
	}
}

func TestCanConsumeInvalidStorage(t *testing.T) {
	access := WhatsAppAccess{StorageLimitGB: "bad", StorageUsedGB: "bad"}
	if access.CanConsume(0, 0, 0, 0) {
		t.Fatal("expected invalid storage values to deny")
	}
	if access.WithinQuota() {
		t.Fatal("expected WithinQuota to deny invalid storage")
	}
}
