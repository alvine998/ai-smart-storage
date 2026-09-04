ALTER TABLE documents MODIFY uploaded_via ENUM('whatsapp', 'web', 'telegram') NOT NULL DEFAULT 'web';
