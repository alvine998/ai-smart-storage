package database

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	phoneutil "ai-smart-storage/internal/phone"
)

var ErrWhatsAppAccessNotFound = errors.New("WhatsApp access not found")
var ErrQuotaExceeded = errors.New("quota exceeded")

const WhatsAppGracePeriod = 3 * 24 * time.Hour

type WhatsAppAccess struct {
	UserID         uint64
	SubscriptionID uint64
	Status         string
	PeriodEnd      time.Time
	StorageLimitGB string
	StorageUsedGB  string
	AIDocsLimit    uint64
	AIDocsUsed     uint64
	AIQueryLimit   uint64
	AIQueriesUsed  uint64
	WALimit        uint64
	WAUsed         uint64
}

func (a WhatsAppAccess) WithinQuota() bool {
	usedStorage, err := strconv.ParseFloat(a.StorageUsedGB, 64)
	if err != nil {
		return false
	}
	storageLimit, err := strconv.ParseFloat(a.StorageLimitGB, 64)
	if err != nil {
		return false
	}
	return usedStorage < storageLimit && a.AIDocsUsed < a.AIDocsLimit && a.AIQueriesUsed < a.AIQueryLimit && a.WAUsed < a.WALimit
}

func (a WhatsAppAccess) InGracePeriod(now time.Time) bool {
	return !a.PeriodEnd.IsZero() && now.After(a.PeriodEnd)
}

// CanConsume checks whether the access can consume additional quota without exceeding limits.
func (a WhatsAppAccess) CanConsume(storageGB float64, aiDocs, aiQueries, waMessages uint64) bool {
	usedStorage, err := strconv.ParseFloat(a.StorageUsedGB, 64)
	if err != nil {
		return false
	}
	storageLimit, err := strconv.ParseFloat(a.StorageLimitGB, 64)
	if err != nil {
		return false
	}
	if usedStorage+storageGB >= storageLimit {
		return false
	}
	if a.AIDocsUsed+aiDocs >= a.AIDocsLimit {
		return false
	}
	if a.AIQueriesUsed+aiQueries >= a.AIQueryLimit {
		return false
	}
	if a.WAUsed+waMessages >= a.WALimit {
		return false
	}
	return true
}

func (s *Store) WhatsAppAccess(ctx context.Context, phone string, now time.Time) (WhatsAppAccess, error) {
	phone = phoneutil.Normalize(phone)
	// Handle legacy records that still have '+' prefix: match either normalized or '+'-prefixed form.
	phonePlus := "+" + phone
	var access WhatsAppAccess
	var periodEnd sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT u.id, sub.id, sub.status, sub.current_period_end,
		       p.storage_quota_gb, COALESCE(q.storage_used_gb, 0),
		       p.ai_docs_quota, COALESCE(q.ai_docs_used, 0),
		       p.ai_query_quota, COALESCE(q.ai_queries_used, 0),
		       p.wa_message_quota, COALESCE(q.wa_messages_used, 0)
		FROM users u
		JOIN subscriptions sub ON sub.user_id = u.id
		JOIN plans p ON p.id = sub.plan_id
		LEFT JOIN usage_quota q ON q.user_id = u.id
			AND q.period_start = sub.current_period_start
			AND q.period_end = sub.current_period_end
		WHERE (u.phone_number = ? OR u.phone_number = ?)
		  AND sub.status = 'active'
		  AND sub.current_period_end > ?
		ORDER BY sub.current_period_end DESC
		LIMIT 1`, phone, phonePlus, now.Add(-WhatsAppGracePeriod)).Scan(
		&access.UserID, &access.SubscriptionID, &access.Status, &periodEnd,
		&access.StorageLimitGB, &access.StorageUsedGB,
		&access.AIDocsLimit, &access.AIDocsUsed,
		&access.AIQueryLimit, &access.AIQueriesUsed,
		&access.WALimit, &access.WAUsed,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return WhatsAppAccess{}, ErrWhatsAppAccessNotFound
	}
	if err != nil {
		return WhatsAppAccess{}, err
	}
	if periodEnd.Valid {
		access.PeriodEnd = periodEnd.Time
	}
	return access, nil
}

// QuotaByUserID returns the same quota view as WhatsAppAccess but looked up by user ID.
// Useful for HTTP handlers that already have an authenticated user ID.
func (s *Store) QuotaByUserID(ctx context.Context, userID uint64, now time.Time) (WhatsAppAccess, error) {
	var access WhatsAppAccess
	var periodEnd sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT u.id, sub.id, sub.status, sub.current_period_end,
		       p.storage_quota_gb, COALESCE(q.storage_used_gb, 0),
		       p.ai_docs_quota, COALESCE(q.ai_docs_used, 0),
		       p.ai_query_quota, COALESCE(q.ai_queries_used, 0),
		       p.wa_message_quota, COALESCE(q.wa_messages_used, 0)
		FROM users u
		JOIN subscriptions sub ON sub.user_id = u.id
		JOIN plans p ON p.id = sub.plan_id
		LEFT JOIN usage_quota q ON q.user_id = u.id
			AND q.period_start = sub.current_period_start
			AND q.period_end = sub.current_period_end
		WHERE u.id = ?
		  AND sub.status = 'active'
		  AND sub.current_period_end > ?
		ORDER BY sub.current_period_end DESC
		LIMIT 1`, userID, now.Add(-WhatsAppGracePeriod)).Scan(
		&access.UserID, &access.SubscriptionID, &access.Status, &periodEnd,
		&access.StorageLimitGB, &access.StorageUsedGB,
		&access.AIDocsLimit, &access.AIDocsUsed,
		&access.AIQueryLimit, &access.AIQueriesUsed,
		&access.WALimit, &access.WAUsed,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return WhatsAppAccess{}, ErrWhatsAppAccessNotFound
	}
	if err != nil {
		return WhatsAppAccess{}, err
	}
	if periodEnd.Valid {
		access.PeriodEnd = periodEnd.Time
	}
	return access, nil
}

// CheckQuota verifies that consuming the given deltas would stay within quota.
// Returns ErrQuotaExceeded if limits would be exceeded.
func (s *Store) CheckQuota(ctx context.Context, userID uint64, storageGB float64, aiDocs, aiQueries, waMessages uint64) error {
	now := time.Now().UTC()
	access, err := s.QuotaByUserID(ctx, userID, now)
	if err != nil {
		return err
	}
	if !access.CanConsume(storageGB, aiDocs, aiQueries, waMessages) {
		return ErrQuotaExceeded
	}
	return nil
}
