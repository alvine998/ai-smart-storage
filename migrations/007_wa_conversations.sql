CREATE TABLE IF NOT EXISTS wa_conversation_windows (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT UNSIGNED NOT NULL,
    window_opened_at DATETIME NOT NULL,
    window_expires_at DATETIME NOT NULL,
    CONSTRAINT fk_wa_windows_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    INDEX idx_wa_windows_user_expiry (user_id, window_expires_at)
);

CREATE TABLE IF NOT EXISTS wa_conversations (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT UNSIGNED NOT NULL,
    wa_message_id VARCHAR(128) NULL,
    direction ENUM('inbound', 'outbound') NOT NULL,
    message_type ENUM('text', 'template', 'media') NOT NULL,
    category ENUM('service', 'utility', 'marketing') NOT NULL,
    content LONGTEXT NULL,
    cost DECIMAL(12, 8) NOT NULL DEFAULT 0.00000000,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_wa_conversations_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    INDEX idx_wa_conversations_user_created (user_id, created_at),
    INDEX idx_wa_conversations_category (category)
);