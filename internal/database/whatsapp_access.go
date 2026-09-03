package database

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"
)

var ErrWhatsAppAccessNotFound = errors.New("WhatsApp access not found")

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

func (s *Store) WhatsAppAccess(ctx context.Context, phone string, now time.Time) (WhatsAppAccess, error) {
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
		WHERE u.phone_number = ?
		  AND sub.status = 'active'
		  AND sub.current_period_end > ?
		ORDER BY sub.current_period_end DESC
		LIMIT 1`, phone, now.Add(-WhatsAppGracePeriod)).Scan(
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
