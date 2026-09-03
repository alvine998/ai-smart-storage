package database

import (
	"context"
	"database/sql"
	"errors"

	"ai-smart-storage/internal/phone"
)

var ErrUserNotFound = errors.New("user not found")

type User struct {
	ID           uint64 `json:"id"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	PasswordHash string `json:"-"`
	PhoneNumber  string `json:"phone_number,omitempty"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

func (s *Store) CreateUser(ctx context.Context, user User) (User, error) {
	user.PhoneNumber = phone.Normalize(user.PhoneNumber)
	result, err := s.db.ExecContext(ctx, `INSERT INTO users (name, email, password_hash, phone_number) VALUES (?, ?, ?, NULLIF(?, ''))`, user.Name, user.Email, user.PasswordHash, user.PhoneNumber)
	if err != nil {
		return User{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return User{}, err
	}
	return s.User(ctx, uint64(id))
}

func (s *Store) Users(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, email, phone_number, created_at, updated_at FROM users ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := make([]User, 0)
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Store) UserByEmail(ctx context.Context, email string) (User, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, email, password_hash, phone_number, created_at, updated_at FROM users WHERE email = ?`, email)
	var user User
	var phoneNumber sql.NullString
	err := row.Scan(&user.ID, &user.Name, &user.Email, &user.PasswordHash, &phoneNumber, &user.CreatedAt, &user.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if phoneNumber.Valid {
		user.PhoneNumber = phoneNumber.String
	}
	return user, err
}

func (s *Store) User(ctx context.Context, id uint64) (User, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, email, phone_number, created_at, updated_at FROM users WHERE id = ?`, id)
	user, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	return user, err
}

func (s *Store) UpdateUser(ctx context.Context, user User) (User, error) {
	user.PhoneNumber = phone.Normalize(user.PhoneNumber)
	result, err := s.db.ExecContext(ctx, `UPDATE users SET name = ?, email = ?, phone_number = NULLIF(?, '') WHERE id = ?`, user.Name, user.Email, user.PhoneNumber, user.ID)
	if err != nil {
		return User{}, err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return User{}, ErrUserNotFound
	}
	return s.User(ctx, user.ID)
}

func (s *Store) UpdatePassword(ctx context.Context, id uint64, passwordHash string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, id)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (s *Store) DeleteUser(ctx context.Context, id uint64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return ErrUserNotFound
	}
	return nil
}

type rowScanner interface{ Scan(...any) error }

func scanUser(row rowScanner) (User, error) {
	var user User
	var phoneNumber sql.NullString
	err := row.Scan(&user.ID, &user.Name, &user.Email, &phoneNumber, &user.CreatedAt, &user.UpdatedAt)
	if phoneNumber.Valid {
		user.PhoneNumber = phoneNumber.String
	}
	return user, err
}
