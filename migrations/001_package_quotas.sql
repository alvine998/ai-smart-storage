ALTER TABLE packages
    ADD COLUMN storage_limit_bytes BIGINT UNSIGNED NOT NULL DEFAULT 0,
    ADD COLUMN ai_token_limit BIGINT UNSIGNED NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS user_usage (
    user_id BIGINT UNSIGNED NOT NULL,
    usage_month DATE NOT NULL,
    storage_bytes BIGINT UNSIGNED NOT NULL DEFAULT 0,
    ai_tokens BIGINT UNSIGNED NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, usage_month),
    CONSTRAINT fk_user_usage_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
