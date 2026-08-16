ALTER TABLE applications ADD COLUMN manifest_service TEXT NOT NULL DEFAULT 'web';
ALTER TABLE applications ADD COLUMN manifest_port INTEGER NOT NULL DEFAULT 80;
