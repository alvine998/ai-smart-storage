ALTER TABLE users ADD COLUMN telegram_chat_id BIGINT NULL UNIQUE;
CREATE INDEX idx_users_telegram_chat ON users(telegram_chat_id);
