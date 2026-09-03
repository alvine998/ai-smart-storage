package database

import (
	"context"
	"database/sql"
	"errors"

	"ai-smart-storage/internal/phone"
)

var ErrBusinessNotFound = errors.New("business not found")

type Business struct {
	ID          uint64 `json:"id"`
	UserID      uint64 `json:"user_id"`
	LegalName   string `json:"legal_name"`
	DisplayName string `json:"display_name,omitempty"`
	TaxID       string `json:"tax_id,omitempty"`
	PhoneNumber string `json:"phone_number,omitempty"`
	Email       string `json:"email,omitempty"`
	Website     string `json:"website,omitempty"`
	Address     string `json:"address,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func (s *Store) CreateBusiness(ctx context.Context, business Business) (Business, error) {
	business.PhoneNumber = phone.Normalize(business.PhoneNumber)
	result, err := s.db.ExecContext(ctx, `INSERT INTO businesses (user_id, legal_name, display_name, tax_id, phone_number, email, website, address) VALUES (?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''))`, business.UserID, business.LegalName, business.DisplayName, business.TaxID, business.PhoneNumber, business.Email, business.Website, business.Address)
	if err != nil {
		return Business{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Business{}, err
	}
	return s.Business(ctx, business.UserID, uint64(id))
}

func (s *Store) Business(ctx context.Context, userID, id uint64) (Business, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, user_id, legal_name, display_name, tax_id, phone_number, email, website, address, created_at, updated_at FROM businesses WHERE user_id = ? AND id = ?`, userID, id)
	business, err := scanBusiness(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Business{}, ErrBusinessNotFound
	}
	return business, err
}

func (s *Store) UserBusiness(ctx context.Context, userID uint64) (Business, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, user_id, legal_name, display_name, tax_id, phone_number, email, website, address, created_at, updated_at FROM businesses WHERE user_id = ?`, userID)
	business, err := scanBusiness(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Business{}, ErrBusinessNotFound
	}
	return business, err
}

func (s *Store) UpdateBusiness(ctx context.Context, business Business) (Business, error) {
	business.PhoneNumber = phone.Normalize(business.PhoneNumber)
	result, err := s.db.ExecContext(ctx, `UPDATE businesses SET legal_name = ?, display_name = NULLIF(?, ''), tax_id = NULLIF(?, ''), phone_number = NULLIF(?, ''), email = NULLIF(?, ''), website = NULLIF(?, ''), address = NULLIF(?, '') WHERE user_id = ? AND id = ?`, business.LegalName, business.DisplayName, business.TaxID, business.PhoneNumber, business.Email, business.Website, business.Address, business.UserID, business.ID)
	if err != nil {
		return Business{}, err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return Business{}, ErrBusinessNotFound
	}
	return s.Business(ctx, business.UserID, business.ID)
}

func (s *Store) DeleteBusiness(ctx context.Context, userID, id uint64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM businesses WHERE user_id = ? AND id = ?`, userID, id)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return ErrBusinessNotFound
	}
	return nil
}

func scanBusiness(row rowScanner) (Business, error) {
	var business Business
	var displayName, taxID, phoneNumber, email, website, address sql.NullString
	err := row.Scan(&business.ID, &business.UserID, &business.LegalName, &displayName, &taxID, &phoneNumber, &email, &website, &address, &business.CreatedAt, &business.UpdatedAt)
	if displayName.Valid {
		business.DisplayName = displayName.String
	}
	if taxID.Valid {
		business.TaxID = taxID.String
	}
	if phoneNumber.Valid {
		business.PhoneNumber = phoneNumber.String
	}
	if email.Valid {
		business.Email = email.String
	}
	if website.Valid {
		business.Website = website.String
	}
	if address.Valid {
		business.Address = address.String
	}
	return business, err
}
