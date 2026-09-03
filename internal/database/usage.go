package database

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrQuotaExceeded = errors.New("quota exceeded")

type Limits struct {
	StorageBytes uint64
	AITokens     uint64
}

func (s *Store) UserLimits(ctx context.Context, userID uint64) (Limits, error) {
	var limits Limits
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(p.storage_limit_bytes), 0), COALESCE(SUM(p.ai_token_limit), 0) FROM user_packages up JOIN packages p ON p.id = up.package_id WHERE up.user_id = ? AND up.status = 'active' AND (up.expires_at IS NULL OR up.expires_at > NOW())`, userID).Scan(&limits.StorageBytes, &limits.AITokens)
	return limits, err
}

func (s *Store) ReserveStorage(ctx context.Context, userID, bytes uint64) error {
	return s.reserve(ctx, userID, bytes, 0)
}

func (s *Store) ReserveAITokens(ctx context.Context, userID, tokens uint64) error {
	return s.reserve(ctx, userID, 0, tokens)
}

func (s *Store) reserve(ctx context.Context, userID, storageBytes, aiTokens uint64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var limits Limits
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(p.storage_limit_bytes), 0), COALESCE(SUM(p.ai_token_limit), 0) FROM user_packages up JOIN packages p ON p.id = up.package_id WHERE up.user_id = ? AND up.status = 'active' AND (up.expires_at IS NULL OR up.expires_at > NOW())`, userID).Scan(&limits.StorageBytes, &limits.AITokens); err != nil {
		return err
	}
	month := time.Now().UTC().Format("2006-01") + "-01"
	var used Limits
	if err := tx.QueryRowContext(ctx, `SELECT storage_bytes, ai_tokens FROM user_usage WHERE user_id = ? AND usage_month = ? FOR UPDATE`, userID, month).Scan(&used.StorageBytes, &used.AITokens); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if (limits.StorageBytes > 0 && used.StorageBytes+storageBytes > limits.StorageBytes) || (limits.AITokens > 0 && used.AITokens+aiTokens > limits.AITokens) {
		return ErrQuotaExceeded
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO user_usage (user_id, usage_month, storage_bytes, ai_tokens) VALUES (?, ?, ?, ?) ON DUPLICATE KEY UPDATE storage_bytes = storage_bytes + VALUES(storage_bytes), ai_tokens = ai_tokens + VALUES(ai_tokens)`, userID, month, storageBytes, aiTokens)
	if err != nil {
		return err
	}
	return tx.Commit()
}
