package database

import (
	"context"
	"database/sql"
	"errors"
)

var ErrDocumentNotFound = errors.New("document not found")
var ErrDocumentTagNotFound = errors.New("document tag not found")

type Document struct {
	ID          uint64 `json:"id"`
	UserID      uint64 `json:"user_id"`
	FileName    string `json:"file_name"`
	R2Key       string `json:"r2_key"`
	FileSize    uint64 `json:"file_size"`
	MimeType    string `json:"mime_type"`
	Category    string `json:"category,omitempty"`
	Summary     string `json:"summary,omitempty"`
	Metadata    string `json:"metadata,omitempty"`
	UploadedVia string `json:"uploaded_via"`
	CreatedAt   string `json:"created_at"`
	DeletedAt   string `json:"deleted_at,omitempty"`
}

type DocumentTag struct {
	ID              uint64  `json:"id"`
	DocumentID      uint64  `json:"document_id"`
	Tag             string  `json:"tag"`
	ConfidenceScore *string `json:"confidence_score,omitempty"`
}

type DocumentVersion struct {
	ID            uint64 `json:"id"`
	DocumentID    uint64 `json:"document_id"`
	VersionNumber uint64 `json:"version_number"`
	R2Key         string `json:"r2_key"`
	CreatedAt     string `json:"created_at"`
}

func (s *Store) CreateDocument(ctx context.Context, document Document) (Document, error) {
	var result Document
	err := s.WithTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		// Insert document
		res, err := tx.ExecContext(ctx, `INSERT INTO documents (user_id, file_name, r2_key, file_size, mime_type, category, summary, metadata, uploaded_via) VALUES (?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?)`, document.UserID, document.FileName, document.R2Key, document.FileSize, document.MimeType, document.Category, document.Summary, document.Metadata, document.UploadedVia)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		// Insert document version
		if _, err = tx.ExecContext(ctx, `INSERT INTO document_versions (document_id, version_number, r2_key) VALUES (?, 1, ?)`, id, document.R2Key); err != nil {
			return err
		}
		// Fetch created document
		row := tx.QueryRowContext(ctx, `SELECT id, user_id, file_name, r2_key, file_size, mime_type, category, summary, metadata, uploaded_via, created_at, deleted_at FROM documents WHERE id = ? AND deleted_at IS NULL`, id)
		result, err = scanDocument(row)
		return err
	})
	if err != nil {
		return Document{}, err
	}
	return result, nil
}

func (s *Store) Documents(ctx context.Context, userID uint64, limit int, offset int) ([]Document, error) {
	if limit <= 0 {
		limit = 20 // default page size
	}
	if limit > 100 {
		limit = 100 // max page size
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, user_id, file_name, r2_key, file_size, mime_type, category, summary, metadata, uploaded_via, created_at, deleted_at FROM documents WHERE user_id = ? AND deleted_at IS NULL ORDER BY id DESC LIMIT ? OFFSET ?`, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Document, 0)
	for rows.Next() {
		item, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) Document(ctx context.Context, id uint64) (Document, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, user_id, file_name, r2_key, file_size, mime_type, category, summary, metadata, uploaded_via, created_at, deleted_at FROM documents WHERE id = ? AND deleted_at IS NULL`, id)
	item, err := scanDocument(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Document{}, ErrDocumentNotFound
	}
	return item, err
}

func (s *Store) UpdateDocument(ctx context.Context, document Document) (Document, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE documents SET category = NULLIF(?, ''), summary = NULLIF(?, ''), metadata = NULLIF(?, '') WHERE id = ? AND deleted_at IS NULL`, document.Category, document.Summary, document.Metadata, document.ID)
	if err != nil {
		return Document{}, err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return Document{}, ErrDocumentNotFound
	}
	return s.Document(ctx, document.ID)
}

func (s *Store) SoftDeleteDocument(ctx context.Context, id uint64) error {
	result, err := s.db.ExecContext(ctx, `UPDATE documents SET deleted_at = CURRENT_TIMESTAMP WHERE id = ? AND deleted_at IS NULL`, id)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return ErrDocumentNotFound
	}
	return nil
}

func (s *Store) DocumentTags(ctx context.Context, documentID uint64) ([]DocumentTag, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, document_id, tag, confidence_score FROM document_tags WHERE document_id = ? ORDER BY id`, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]DocumentTag, 0)
	for rows.Next() {
		var item DocumentTag
		var score sql.NullString
		if err := rows.Scan(&item.ID, &item.DocumentID, &item.Tag, &score); err != nil {
			return nil, err
		}
		if score.Valid {
			item.ConfidenceScore = &score.String
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) CreateDocumentTag(ctx context.Context, tag DocumentTag) (DocumentTag, error) {
	result, err := s.db.ExecContext(ctx, `INSERT INTO document_tags (document_id, tag, confidence_score) VALUES (?, ?, NULLIF(?, ''))`, tag.DocumentID, tag.Tag, valueOrEmpty(tag.ConfidenceScore))
	if err != nil {
		return DocumentTag{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return DocumentTag{}, err
	}
	var item DocumentTag
	var score sql.NullString
	err = s.db.QueryRowContext(ctx, `SELECT id, document_id, tag, confidence_score FROM document_tags WHERE id = ?`, id).Scan(&item.ID, &item.DocumentID, &item.Tag, &score)
	if score.Valid {
		item.ConfidenceScore = &score.String
	}
	return item, err
}

func (s *Store) DeleteDocumentTag(ctx context.Context, documentID, tagID uint64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM document_tags WHERE document_id = ? AND id = ?`, documentID, tagID)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return ErrDocumentTagNotFound
	}
	return nil
}

func (s *Store) DocumentVersions(ctx context.Context, documentID uint64) ([]DocumentVersion, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, document_id, version_number, r2_key, created_at FROM document_versions WHERE document_id = ? ORDER BY version_number DESC`, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]DocumentVersion, 0)
	for rows.Next() {
		var item DocumentVersion
		if err := rows.Scan(&item.ID, &item.DocumentID, &item.VersionNumber, &item.R2Key, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanDocument(row rowScanner) (Document, error) {
	var item Document
	var category, summary, metadata, deletedAt sql.NullString
	err := row.Scan(&item.ID, &item.UserID, &item.FileName, &item.R2Key, &item.FileSize, &item.MimeType, &category, &summary, &metadata, &item.UploadedVia, &item.CreatedAt, &deletedAt)
	if category.Valid {
		item.Category = category.String
	}
	if summary.Valid {
		item.Summary = summary.String
	}
	if metadata.Valid {
		item.Metadata = metadata.String
	}
	if deletedAt.Valid {
		item.DeletedAt = deletedAt.String
	}
	return item, err
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
