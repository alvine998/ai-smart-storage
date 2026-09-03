-- Add index on users.phone_number for faster lookups in WhatsApp flow
ALTER TABLE users ADD INDEX idx_users_phone_number (phone_number);

-- Modify wa_conversation_windows to use UNIQUE constraint for upsert pattern
-- Remove old index and add unique constraint to ensure only one active window per user
ALTER TABLE wa_conversation_windows DROP INDEX idx_wa_windows_user_expiry;
ALTER TABLE wa_conversation_windows ADD UNIQUE KEY uq_wa_windows_user (user_id);
ALTER TABLE wa_conversation_windows ADD INDEX idx_wa_windows_expiry (window_expires_at);
