package database

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrUsageQuotaNotFound = errors.New("usage quota not found")

type UsageQuota struct {
	ID             uint64 `json:"id"`
	UserID         uint64 `json:"user_id"`
	PeriodStart    string `json:"period_start"`
	PeriodEnd      string `json:"period_end"`
	StorageUsedGB  string `json:"storage_used_gb"`
	AIDocsUsed     uint64 `json:"ai_docs_used"`
	AIQueriesUsed  uint64 `json:"ai_queries_used"`
	WAMessagesUsed uint64 `json:"wa_messages_used"`
	UpdatedAt      string `json:"updated_at"`
}

func (s *Store) CurrentUsageQuota(ctx context.Context, userID uint64) (UsageQuota, error) {
	var item UsageQuota
	err := s.db.QueryRowContext(ctx, `SELECT id, user_id, period_start, period_end, storage_used_gb, ai_docs_used, ai_queries_used, wa_messages_used, updated_at FROM usage_quota WHERE user_id = ? AND period_start <= NOW() AND period_end > NOW() ORDER BY period_end DESC LIMIT 1`, userID).Scan(&item.ID, &item.UserID, &item.PeriodStart, &item.PeriodEnd, &item.StorageUsedGB, &item.AIDocsUsed, &item.AIQueriesUsed, &item.WAMessagesUsed, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return UsageQuota{}, ErrUsageQuotaNotFound
	}
	return item, err
}

func (s *Store) IncrementUsageQuota(ctx context.Context, userID uint64, storageGB string, aiDocs, aiQueries, waMessages uint64) error {
	start, end, err := s.usagePeriod(ctx, userID)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO usage_quota (user_id, period_start, period_end, storage_used_gb, ai_docs_used, ai_queries_used, wa_messages_used) VALUES (?, ?, ?, ?, ?, ?, ?) ON DUPLICATE KEY UPDATE storage_used_gb = storage_used_gb + VALUES(storage_used_gb), ai_docs_used = ai_docs_used + VALUES(ai_docs_used), ai_queries_used = ai_queries_used + VALUES(ai_queries_used), wa_messages_used = wa_messages_used + VALUES(wa_messages_used)`, userID, start, end, storageGB, aiDocs, aiQueries, waMessages)
	return err
}

func (s *Store) usagePeriod(ctx context.Context, userID uint64) (time.Time, time.Time, error) {
	var start, end time.Time
	err := s.db.QueryRowContext(ctx, `SELECT current_period_start, current_period_end FROM subscriptions WHERE user_id = ? AND status = 'active' AND current_period_end > NOW() ORDER BY current_period_end DESC LIMIT 1`, userID).Scan(&start, &end)
	if errors.Is(err, sql.ErrNoRows) {
		now := time.Now().UTC()
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		end = start.AddDate(0, 1, 0)
		return start, end, nil
	}
	return start, end, err
}
