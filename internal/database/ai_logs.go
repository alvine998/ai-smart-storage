package database

import (
	"context"
	"database/sql"
)

type AIProcessingLog struct {
	ID            uint64  `json:"id"`
	UserID        uint64  `json:"user_id"`
	DocumentID    *uint64 `json:"document_id,omitempty"`
	ActionType    string  `json:"action_type"`
	InputTokens   uint64  `json:"input_tokens"`
	OutputTokens  uint64  `json:"output_tokens"`
	EstimatedCost string  `json:"estimated_cost"`
	CreatedAt     string  `json:"created_at"`
}

func (s *Store) CreateAIProcessingLog(ctx context.Context, logEntry AIProcessingLog) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO ai_processing_logs (user_id, document_id, action_type, input_tokens, output_tokens, estimated_cost) VALUES (?, ?, ?, ?, ?, ?)`, logEntry.UserID, logEntry.DocumentID, logEntry.ActionType, logEntry.InputTokens, logEntry.OutputTokens, logEntry.EstimatedCost)
	return err
}

func (s *Store) AIProcessingLogs(ctx context.Context, userID uint64) ([]AIProcessingLog, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, user_id, document_id, action_type, input_tokens, output_tokens, estimated_cost, created_at FROM ai_processing_logs WHERE user_id = ? ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]AIProcessingLog, 0)
	for rows.Next() {
		var item AIProcessingLog
		var documentID sql.NullInt64
		if err := rows.Scan(&item.ID, &item.UserID, &documentID, &item.ActionType, &item.InputTokens, &item.OutputTokens, &item.EstimatedCost, &item.CreatedAt); err != nil {
			return nil, err
		}
		if documentID.Valid {
			value := uint64(documentID.Int64)
			item.DocumentID = &value
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
