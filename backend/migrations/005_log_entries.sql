CREATE TABLE IF NOT EXISTS log_entries (
  id TEXT PRIMARY KEY,
  occurred_at TEXT NOT NULL,
  level TEXT NOT NULL,
  component TEXT NOT NULL,
  application_id TEXT REFERENCES applications(id) ON DELETE CASCADE,
  operation_id TEXT REFERENCES operations(id) ON DELETE SET NULL,
  service TEXT NOT NULL DEFAULT '',
  container_name TEXT NOT NULL DEFAULT '',
  message TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS log_entries_application_order ON log_entries(application_id, occurred_at, id);
CREATE INDEX IF NOT EXISTS log_entries_operation_order ON log_entries(operation_id, occurred_at, id);
CREATE INDEX IF NOT EXISTS log_entries_service_order ON log_entries(application_id, service, occurred_at, id);
CREATE INDEX IF NOT EXISTS log_entries_order ON log_entries(occurred_at, id);
