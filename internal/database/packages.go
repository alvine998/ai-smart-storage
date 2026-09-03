package database

import (
	"context"
	"database/sql"
	"errors"
)

var ErrPackageNotFound = errors.New("package not found")
var ErrUserPackageNotFound = errors.New("user package not found")

type Package struct {
	ID          uint64 `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Price       string `json:"price"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func (s *Store) CreatePackage(ctx context.Context, item Package) (Package, error) {
	result, err := s.db.ExecContext(ctx, `INSERT INTO packages (name, description, price) VALUES (?, NULLIF(?, ''), ?)`, item.Name, item.Description, item.Price)
	if err != nil {
		return Package{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Package{}, err
	}
	return s.Package(ctx, uint64(id))
}

func (s *Store) Packages(ctx context.Context) ([]Package, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, description, price, created_at, updated_at FROM packages ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Package, 0)
	for rows.Next() {
		item, err := scanPackage(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) Package(ctx context.Context, id uint64) (Package, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, description, price, created_at, updated_at FROM packages WHERE id = ?`, id)
	item, err := scanPackage(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Package{}, ErrPackageNotFound
	}
	return item, err
}

func (s *Store) UpdatePackage(ctx context.Context, item Package) (Package, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE packages SET name = ?, description = NULLIF(?, ''), price = ? WHERE id = ?`, item.Name, item.Description, item.Price, item.ID)
	if err != nil {
		return Package{}, err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return Package{}, ErrPackageNotFound
	}
	return s.Package(ctx, item.ID)
}

func (s *Store) DeletePackage(ctx context.Context, id uint64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM packages WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return ErrPackageNotFound
	}
	return nil
}

type UserPackage struct {
	ID        uint64 `json:"id"`
	UserID    uint64 `json:"user_id"`
	PackageID uint64 `json:"package_id"`
	Status    string `json:"status"`
	ExpiresAt string `json:"expires_at,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func (s *Store) CreateUserPackage(ctx context.Context, item UserPackage) (UserPackage, error) {
	result, err := s.db.ExecContext(ctx, `INSERT INTO user_packages (user_id, package_id, status, expires_at) VALUES (?, ?, ?, NULLIF(?, ''))`, item.UserID, item.PackageID, item.Status, item.ExpiresAt)
	if err != nil {
		return UserPackage{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return UserPackage{}, err
	}
	return s.UserPackage(ctx, item.UserID, uint64(id))
}

func (s *Store) UserPackages(ctx context.Context, userID uint64) ([]UserPackage, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, user_id, package_id, status, expires_at, created_at, updated_at FROM user_packages WHERE user_id = ? ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]UserPackage, 0)
	for rows.Next() {
		item, err := scanUserPackage(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) UserPackage(ctx context.Context, userID, id uint64) (UserPackage, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, user_id, package_id, status, expires_at, created_at, updated_at FROM user_packages WHERE user_id = ? AND id = ?`, userID, id)
	item, err := scanUserPackage(row)
	if errors.Is(err, sql.ErrNoRows) {
		return UserPackage{}, ErrUserPackageNotFound
	}
	return item, err
}

func (s *Store) UpdateUserPackage(ctx context.Context, item UserPackage) (UserPackage, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE user_packages SET package_id = ?, status = ?, expires_at = NULLIF(?, '') WHERE user_id = ? AND id = ?`, item.PackageID, item.Status, item.ExpiresAt, item.UserID, item.ID)
	if err != nil {
		return UserPackage{}, err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return UserPackage{}, ErrUserPackageNotFound
	}
	return s.UserPackage(ctx, item.UserID, item.ID)
}

func (s *Store) DeleteUserPackage(ctx context.Context, userID, id uint64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM user_packages WHERE user_id = ? AND id = ?`, userID, id)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return ErrUserPackageNotFound
	}
	return nil
}

func scanPackage(row rowScanner) (Package, error) {
	var item Package
	var description sql.NullString
	err := row.Scan(&item.ID, &item.Name, &description, &item.Price, &item.CreatedAt, &item.UpdatedAt)
	if description.Valid {
		item.Description = description.String
	}
	return item, err
}

func scanUserPackage(row rowScanner) (UserPackage, error) {
	var item UserPackage
	var expiresAt sql.NullString
	err := row.Scan(&item.ID, &item.UserID, &item.PackageID, &item.Status, &expiresAt, &item.CreatedAt, &item.UpdatedAt)
	if expiresAt.Valid {
		item.ExpiresAt = expiresAt.String
	}
	return item, err
}
