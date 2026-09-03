CREATE TABLE IF NOT EXISTS messages (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    wa_message_id VARCHAR(128) NULL UNIQUE,
    phone_number VARCHAR(32) NOT NULL,
    role ENUM('user', 'assistant', 'system') NOT NULL,
    content LONGTEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_messages_phone_created (phone_number, created_at)
);

CREATE TABLE IF NOT EXISTS users (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(120) NOT NULL,
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    phone_number VARCHAR(32) NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS businesses (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT UNSIGNED NOT NULL UNIQUE,
    legal_name VARCHAR(160) NOT NULL,
    display_name VARCHAR(160) NULL,
    tax_id VARCHAR(80) NULL,
    phone_number VARCHAR(32) NULL,
    email VARCHAR(255) NULL,
    website VARCHAR(255) NULL,
    address TEXT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_businesses_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS plans (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    name ENUM('Starter', 'Business', 'Enterprise') NOT NULL UNIQUE,
    price DECIMAL(10, 2) NOT NULL DEFAULT 0.00,
    storage_quota_gb DECIMAL(10, 2) NOT NULL DEFAULT 0.00,
    ai_docs_quota BIGINT UNSIGNED NOT NULL DEFAULT 0,
    ai_query_quota BIGINT UNSIGNED NOT NULL DEFAULT 0,
    wa_message_quota BIGINT UNSIGNED NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS subscriptions (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT UNSIGNED NOT NULL,
    plan_id BIGINT UNSIGNED NOT NULL,
    status ENUM('active', 'past_due', 'canceled') NOT NULL DEFAULT 'active',
    current_period_start DATETIME NOT NULL,
    current_period_end DATETIME NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_subscriptions_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT fk_subscriptions_plan FOREIGN KEY (plan_id) REFERENCES plans(id) ON DELETE RESTRICT,
    INDEX idx_subscriptions_user (user_id),
    INDEX idx_subscriptions_plan (plan_id)
);

CREATE TABLE IF NOT EXISTS invoices (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT UNSIGNED NOT NULL,
    subscription_id BIGINT UNSIGNED NOT NULL,
    amount DECIMAL(10, 2) NOT NULL DEFAULT 0.00,
    status ENUM('paid', 'pending', 'failed') NOT NULL DEFAULT 'pending',
    payment_method VARCHAR(80) NULL,
    paid_at DATETIME NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_invoices_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT fk_invoices_subscription FOREIGN KEY (subscription_id) REFERENCES subscriptions(id) ON DELETE RESTRICT,
    INDEX idx_invoices_user (user_id),
    INDEX idx_invoices_subscription (subscription_id)
);

CREATE TABLE IF NOT EXISTS documents (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT UNSIGNED NOT NULL,
    file_name VARCHAR(255) NOT NULL,
    r2_key VARCHAR(512) NOT NULL UNIQUE,
    file_size BIGINT UNSIGNED NOT NULL,
    mime_type VARCHAR(255) NOT NULL,
    category VARCHAR(120) NULL,
    summary TEXT NULL,
    metadata JSON NULL,
    uploaded_via ENUM('whatsapp', 'web') NOT NULL DEFAULT 'web',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL,
    CONSTRAINT fk_documents_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    INDEX idx_documents_user_created (user_id, created_at),
    INDEX idx_documents_deleted (deleted_at)
);

CREATE TABLE IF NOT EXISTS document_tags (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    document_id BIGINT UNSIGNED NOT NULL,
    tag VARCHAR(120) NOT NULL,
    confidence_score DECIMAL(5, 4) NULL,
    CONSTRAINT fk_document_tags_document FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE CASCADE,
    UNIQUE KEY uq_document_tag (document_id, tag),
    INDEX idx_document_tags_document (document_id)
);

CREATE TABLE IF NOT EXISTS document_versions (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    document_id BIGINT UNSIGNED NOT NULL,
    version_number INT UNSIGNED NOT NULL,
    r2_key VARCHAR(512) NOT NULL UNIQUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_document_versions_document FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE CASCADE,
    UNIQUE KEY uq_document_version (document_id, version_number)
);

CREATE TABLE IF NOT EXISTS ai_processing_logs (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT UNSIGNED NOT NULL,
    document_id BIGINT UNSIGNED NULL,
    action_type ENUM('categorize', 'summarize', 'search_query') NOT NULL,
    input_tokens BIGINT UNSIGNED NOT NULL DEFAULT 0,
    output_tokens BIGINT UNSIGNED NOT NULL DEFAULT 0,
    estimated_cost DECIMAL(12, 8) NOT NULL DEFAULT 0.00000000,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_ai_logs_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT fk_ai_logs_document FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE SET NULL,
    INDEX idx_ai_logs_user_created (user_id, created_at),
    INDEX idx_ai_logs_document (document_id),
    INDEX idx_ai_logs_action (action_type)
);

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
    INDEX idx_wa_conversations_user_created (user_id, created_at)
);

CREATE TABLE IF NOT EXISTS usage_quotas (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT UNSIGNED NOT NULL UNIQUE,
    ai_documents_processed BIGINT UNSIGNED NOT NULL DEFAULT 0,
    ai_queries_made BIGINT UNSIGNED NOT NULL DEFAULT 0,
    whatsapp_messages_sent BIGINT UNSIGNED NOT NULL DEFAULT 0,
    whatsapp_messages_received BIGINT UNSIGNED NOT NULL DEFAULT 0,
    storage_used_gb DECIMAL(16, 6) NOT NULL DEFAULT 0.000000,
    estimated_cost DECIMAL(12, 8) NOT NULL DEFAULT 0.00000000,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_usage_quotas_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
