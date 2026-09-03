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
    INDEX idx_documents_user_created (user_id),
    INDEX idx_documents_deleted (deleted_at)
);

CREATE TABLE IF NOT EXISTS document_tags (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    document_id BIGINT UNSIGNED NOT NULL,
    tag VARCHAR(120) NOT NULL,
    confidence_score DECIMAL(5, 4) NULL,
    CONSTRAINT fk_document_tags_document FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE CASCADE,
    UNIQUE KEY uq_document_tag (document_id, tag)
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