package database

import (
	"context"
	"database/sql"
	"errors"
)

var ErrPlanNotFound = errors.New("plan not found")
var ErrSubscriptionNotFound = errors.New("subscription not found")
var ErrInvoiceNotFound = errors.New("invoice not found")

type Plan struct {
	ID             uint64 `json:"id"`
	Name           string `json:"name"`
	Price          string `json:"price"`
	StorageQuotaGB string `json:"storage_quota_gb"`
	AIDocsQuota    uint64 `json:"ai_docs_quota"`
	AIQueryQuota   uint64 `json:"ai_query_quota"`
	WAMessageQuota uint64 `json:"wa_message_quota"`
	IsActive       bool   `json:"is_active"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

func (s *Store) CreatePlan(ctx context.Context, plan Plan) (Plan, error) {
	result, err := s.db.ExecContext(ctx, `INSERT INTO plans (name, price, storage_quota_gb, ai_docs_quota, ai_query_quota, wa_message_quota, is_active) VALUES (?, ?, ?, ?, ?, ?, ?)`, plan.Name, plan.Price, plan.StorageQuotaGB, plan.AIDocsQuota, plan.AIQueryQuota, plan.WAMessageQuota, plan.IsActive)
	if err != nil {
		return Plan{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Plan{}, err
	}
	return s.Plan(ctx, uint64(id))
}

func (s *Store) Plans(ctx context.Context) ([]Plan, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, price, storage_quota_gb, ai_docs_quota, ai_query_quota, wa_message_quota, is_active, created_at, updated_at FROM plans ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	plans := make([]Plan, 0)
	for rows.Next() {
		plan, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	return plans, rows.Err()
}

func (s *Store) Plan(ctx context.Context, id uint64) (Plan, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, price, storage_quota_gb, ai_docs_quota, ai_query_quota, wa_message_quota, is_active, created_at, updated_at FROM plans WHERE id = ?`, id)
	plan, err := scanPlan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Plan{}, ErrPlanNotFound
	}
	return plan, err
}

func (s *Store) UpdatePlan(ctx context.Context, plan Plan) (Plan, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE plans SET name = ?, price = ?, storage_quota_gb = ?, ai_docs_quota = ?, ai_query_quota = ?, wa_message_quota = ?, is_active = ? WHERE id = ?`, plan.Name, plan.Price, plan.StorageQuotaGB, plan.AIDocsQuota, plan.AIQueryQuota, plan.WAMessageQuota, plan.IsActive, plan.ID)
	if err != nil {
		return Plan{}, err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return Plan{}, ErrPlanNotFound
	}
	return s.Plan(ctx, plan.ID)
}

func (s *Store) DeletePlan(ctx context.Context, id uint64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM plans WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return ErrPlanNotFound
	}
	return nil
}

type Subscription struct {
	ID                 uint64 `json:"id"`
	UserID             uint64 `json:"user_id"`
	PlanID             uint64 `json:"plan_id"`
	Status             string `json:"status"`
	CurrentPeriodStart string `json:"current_period_start"`
	CurrentPeriodEnd   string `json:"current_period_end"`
	CreatedAt          string `json:"created_at"`
}

func (s *Store) CreateSubscription(ctx context.Context, item Subscription) (Subscription, error) {
	result, err := s.db.ExecContext(ctx, `INSERT INTO subscriptions (user_id, plan_id, status, current_period_start, current_period_end) VALUES (?, ?, ?, ?, ?)`, item.UserID, item.PlanID, item.Status, item.CurrentPeriodStart, item.CurrentPeriodEnd)
	if err != nil {
		return Subscription{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Subscription{}, err
	}
	return s.Subscription(ctx, uint64(id))
}

func (s *Store) Subscriptions(ctx context.Context) ([]Subscription, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, user_id, plan_id, status, current_period_start, current_period_end, created_at FROM subscriptions ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Subscription, 0)
	for rows.Next() {
		item, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) Subscription(ctx context.Context, id uint64) (Subscription, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, user_id, plan_id, status, current_period_start, current_period_end, created_at FROM subscriptions WHERE id = ?`, id)
	item, err := scanSubscription(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Subscription{}, ErrSubscriptionNotFound
	}
	return item, err
}

func (s *Store) UpdateSubscription(ctx context.Context, item Subscription) (Subscription, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE subscriptions SET user_id = ?, plan_id = ?, status = ?, current_period_start = ?, current_period_end = ? WHERE id = ?`, item.UserID, item.PlanID, item.Status, item.CurrentPeriodStart, item.CurrentPeriodEnd, item.ID)
	if err != nil {
		return Subscription{}, err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return Subscription{}, ErrSubscriptionNotFound
	}
	return s.Subscription(ctx, item.ID)
}

func (s *Store) DeleteSubscription(ctx context.Context, id uint64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM subscriptions WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return ErrSubscriptionNotFound
	}
	return nil
}

type Invoice struct {
	ID             uint64 `json:"id"`
	UserID         uint64 `json:"user_id"`
	SubscriptionID uint64 `json:"subscription_id"`
	Amount         string `json:"amount"`
	Status         string `json:"status"`
	PaymentMethod  string `json:"payment_method,omitempty"`
	PaidAt         string `json:"paid_at,omitempty"`
	CreatedAt      string `json:"created_at"`
}

func (s *Store) CreateInvoice(ctx context.Context, item Invoice) (Invoice, error) {
	result, err := s.db.ExecContext(ctx, `INSERT INTO invoices (user_id, subscription_id, amount, status, payment_method, paid_at) VALUES (?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''))`, item.UserID, item.SubscriptionID, item.Amount, item.Status, item.PaymentMethod, item.PaidAt)
	if err != nil {
		return Invoice{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Invoice{}, err
	}
	return s.Invoice(ctx, uint64(id))
}

func (s *Store) Invoices(ctx context.Context) ([]Invoice, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, user_id, subscription_id, amount, status, payment_method, paid_at, created_at FROM invoices ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Invoice, 0)
	for rows.Next() {
		item, err := scanInvoice(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) Invoice(ctx context.Context, id uint64) (Invoice, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, user_id, subscription_id, amount, status, payment_method, paid_at, created_at FROM invoices WHERE id = ?`, id)
	item, err := scanInvoice(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Invoice{}, ErrInvoiceNotFound
	}
	return item, err
}

func (s *Store) UpdateInvoice(ctx context.Context, item Invoice) (Invoice, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE invoices SET user_id = ?, subscription_id = ?, amount = ?, status = ?, payment_method = NULLIF(?, ''), paid_at = NULLIF(?, '') WHERE id = ?`, item.UserID, item.SubscriptionID, item.Amount, item.Status, item.PaymentMethod, item.PaidAt, item.ID)
	if err != nil {
		return Invoice{}, err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return Invoice{}, ErrInvoiceNotFound
	}
	return s.Invoice(ctx, item.ID)
}

func (s *Store) DeleteInvoice(ctx context.Context, id uint64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM invoices WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return ErrInvoiceNotFound
	}
	return nil
}

func scanPlan(row rowScanner) (Plan, error) {
	var plan Plan
	err := row.Scan(&plan.ID, &plan.Name, &plan.Price, &plan.StorageQuotaGB, &plan.AIDocsQuota, &plan.AIQueryQuota, &plan.WAMessageQuota, &plan.IsActive, &plan.CreatedAt, &plan.UpdatedAt)
	return plan, err
}

func scanSubscription(row rowScanner) (Subscription, error) {
	var item Subscription
	err := row.Scan(&item.ID, &item.UserID, &item.PlanID, &item.Status, &item.CurrentPeriodStart, &item.CurrentPeriodEnd, &item.CreatedAt)
	return item, err
}

func scanInvoice(row rowScanner) (Invoice, error) {
	var item Invoice
	var paymentMethod, paidAt sql.NullString
	err := row.Scan(&item.ID, &item.UserID, &item.SubscriptionID, &item.Amount, &item.Status, &paymentMethod, &paidAt, &item.CreatedAt)
	if paymentMethod.Valid {
		item.PaymentMethod = paymentMethod.String
	}
	if paidAt.Valid {
		item.PaidAt = paidAt.String
	}
	return item, err
}
