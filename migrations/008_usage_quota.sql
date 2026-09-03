CREATE TABLE IF NOT EXISTS usage_quota (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT UNSIGNED NOT NULL,
    period_start DATETIME NOT NULL,
    period_end DATETIME NOT NULL,
    storage_used_gb DECIMAL(12, 6) NOT NULL DEFAULT 0.000000,
    ai_docs_used BIGINT UNSIGNED NOT NULL DEFAULT 0,
    ai_queries_used BIGINT UNSIGNED NOT NULL DEFAULT 0,
    wa_messages_used BIGINT UNSIGNED NOT NULL DEFAULT 0,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_usage_quota_period (user_id, period_start, period_end),
    INDEX idx_usage_quota_user_updated (user_id, updated_at),
    CONSTRAINT fk_usage_quota_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);