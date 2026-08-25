ALTER TABLE operations ADD COLUMN phase TEXT NOT NULL DEFAULT 'queued';
ALTER TABLE operations ADD COLUMN display_message TEXT NOT NULL DEFAULT '開始を待機しています';
