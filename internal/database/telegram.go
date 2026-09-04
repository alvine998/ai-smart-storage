package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"ai-smart-storage/internal/phone"
)

func (s *Store) TelegramAccess(ctx context.Context, chatID int64, now time.Time) (WhatsAppAccess, error) {
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
		WHERE u.telegram_chat_id = ?
		  AND sub.status = 'active'
		  AND sub.current_period_end > ?
		ORDER BY sub.current_period_end DESC
		LIMIT 1`, chatID, now.Add(-WhatsAppGracePeriod)).Scan(
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
		return WhatsAppAccess{}, fmt.Errorf("telegram access query: %w", err)
	}
	if periodEnd.Valid {
		access.PeriodEnd = periodEnd.Time
	}
	return access, nil
}

func (s *Store) LinkTelegramChat(ctx context.Context, userID uint64, chatID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET telegram_chat_id = ? WHERE id = ? AND telegram_chat_id IS NULL`, chatID, userID)
	return err
}

func (s *Store) LinkTelegramByPhoneIfNull(ctx context.Context, rawPhone string, chatID int64) (uint64, bool, error) {
	normalized := phone.Normalize(rawPhone)
	phonePlus := "+" + normalized
	var userID uint64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM users WHERE (phone_number = ? OR phone_number = ?) AND telegram_chat_id IS NULL LIMIT 1`, normalized, phonePlus).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE users SET telegram_chat_id = ? WHERE id = ? AND telegram_chat_id IS NULL`, chatID, userID)
	if err != nil {
		return 0, false, err
	}
	return userID, true, nil
}

func (s *Store) UserByTelegramChat(ctx context.Context, chatID int64) (uint64, error) {
	var userID uint64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM users WHERE telegram_chat_id = ?`, chatID).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrWhatsAppAccessNotFound
	}
	return userID, err
}

func (s *Store) UserByPhoneNumber(ctx context.Context, rawPhone string) (User, error) {
	normalized := phone.Normalize(rawPhone)
	phonePlus := "+" + normalized
	var user User
	err := s.db.QueryRowContext(ctx, `SELECT id, name, email, COALESCE(phone_number, '') FROM users WHERE phone_number = ? OR phone_number = ?`, normalized, phonePlus).Scan(&user.ID, &user.Name, &user.Email, &user.PhoneNumber)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	return user, err
}
