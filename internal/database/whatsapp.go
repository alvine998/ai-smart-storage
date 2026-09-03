package database

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrUserPhoneNotFound = errors.New("user phone not found")

type WAConversation struct {
	ID          uint64 `json:"id"`
	UserID      uint64 `json:"user_id"`
	WAMessageID string `json:"wa_message_id,omitempty"`
	Direction   string `json:"direction"`
	MessageType string `json:"message_type"`
	Category    string `json:"category"`
	Content     string `json:"content,omitempty"`
	Cost        string `json:"cost"`
	CreatedAt   string `json:"created_at"`
}

type WAConversationWindow struct {
	ID              uint64 `json:"id"`
	UserID          uint64 `json:"user_id"`
	WindowOpenedAt  string `json:"window_opened_at"`
	WindowExpiresAt string `json:"window_expires_at"`
}

func (s *Store) UserByPhone(ctx context.Context, phone string) (uint64, error) {
	var id uint64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM users WHERE phone_number = ?`, phone).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrUserPhoneNotFound
	}
	return id, err
}

func (s *Store) LogWAConversation(ctx context.Context, conversation WAConversation) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO wa_conversations (user_id, wa_message_id, direction, message_type, category, content, cost) VALUES (?, NULLIF(?, ''), ?, ?, ?, ?, ?)`, conversation.UserID, conversation.WAMessageID, conversation.Direction, conversation.MessageType, conversation.Category, conversation.Content, conversation.Cost)
	return err
}

func (s *Store) OpenWAWindow(ctx context.Context, userID uint64, openedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO wa_conversation_windows (user_id, window_opened_at, window_expires_at) VALUES (?, ?, ?)`, userID, openedAt, openedAt.Add(24*time.Hour))
	return err
}

func (s *Store) WAWindowOpen(ctx context.Context, userID uint64, now time.Time) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM wa_conversation_windows WHERE user_id = ? AND window_expires_at > ? ORDER BY window_expires_at DESC LIMIT 1)`, userID, now).Scan(&exists)
	return exists, err
}

func (s *Store) WAConversations(ctx context.Context, userID uint64) ([]WAConversation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, user_id, wa_message_id, direction, message_type, category, content, cost, created_at FROM wa_conversations WHERE user_id = ? ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]WAConversation, 0)
	for rows.Next() {
		var item WAConversation
		var messageID, content sql.NullString
		if err := rows.Scan(&item.ID, &item.UserID, &messageID, &item.Direction, &item.MessageType, &item.Category, &content, &item.Cost, &item.CreatedAt); err != nil {
			return nil, err
		}
		if messageID.Valid {
			item.WAMessageID = messageID.String
		}
		if content.Valid {
			item.Content = content.String
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
