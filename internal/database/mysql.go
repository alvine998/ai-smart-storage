package database

import (
	"context"
	"database/sql"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type Store struct{ db *sql.DB }

func Open(dsn string) (*Store, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) SaveMessage(ctx context.Context, waID, phone, role, content string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO messages (wa_message_id, phone_number, role, content)
		VALUES (NULLIF(?, ''), ?, ?, ?)
		ON DUPLICATE KEY UPDATE content = VALUES(content)`, waID, phone, role, content)
	return err
}

func (s *Store) History(ctx context.Context, phone string, limit int) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT role, content FROM messages WHERE phone_number = ? ORDER BY created_at DESC LIMIT ?`, phone, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var messages []Message
	for rows.Next() {
		var message Message
		if err := rows.Scan(&message.Role, &message.Content); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
		messages[left], messages[right] = messages[right], messages[left]
	}
	return messages, rows.Err()
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
