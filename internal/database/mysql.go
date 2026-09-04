package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"ai-smart-storage/internal/crypto"
	phoneutil "ai-smart-storage/internal/phone"

	_ "github.com/go-sql-driver/mysql"
)

type Store struct {
	db        *sql.DB
	msgCipher *crypto.Cipher
}

// TxFn is a callback function that executes within a transaction
type TxFn func(context.Context, *sql.Tx) error

// Open connects to MySQL. messageKey enables at-rest encryption of chat
// message content when set; rows written while it was empty stay readable.
func Open(dsn, messageKey string) (*Store, error) {
	msgCipher, err := crypto.NewCipher(messageKey)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, msgCipher: msgCipher}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Ping(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("store not initialized")
	}
	return s.db.PingContext(ctx)
}

// WithTx executes a callback function within a database transaction
func (s *Store) WithTx(ctx context.Context, fn TxFn) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(ctx, tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) SaveMessage(ctx context.Context, waID, phone, role, content string) error {
	phone = phoneutil.Normalize(phone)
	content, err := s.msgCipher.Encrypt(content)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO messages (wa_message_id, phone_number, role, content)
		VALUES (NULLIF(?, ''), ?, ?, ?)
		ON DUPLICATE KEY UPDATE content = VALUES(content)`, waID, phone, role, content)
	return err
}

func (s *Store) History(ctx context.Context, phone string, limit int) ([]Message, error) {
	phone = phoneutil.Normalize(phone)
	phonePlus := "+" + phone
	rows, err := s.db.QueryContext(ctx, `SELECT role, content FROM messages WHERE (phone_number = ? OR phone_number = ?) ORDER BY created_at DESC LIMIT ?`, phone, phonePlus, limit)
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
		if message.Content, err = s.msgCipher.Decrypt(message.Content); err != nil {
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
