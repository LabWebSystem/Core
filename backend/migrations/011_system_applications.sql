ALTER TABLE applications ADD COLUMN is_system INTEGER NOT NULL DEFAULT 0;

CREATE TRIGGER IF NOT EXISTS applications_protect_system_delete
BEFORE DELETE ON applications
WHEN OLD.is_system = 1
BEGIN
  SELECT RAISE(ABORT, 'system applications cannot be deleted');
END;
