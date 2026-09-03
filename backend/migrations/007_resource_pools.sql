CREATE TABLE IF NOT EXISTS lws_devices (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  stable_id TEXT NOT NULL UNIQUE,
  current_path TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'DISCONNECTED',
  metadata TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS application_device_bindings (
  application_id TEXT NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
  service TEXT NOT NULL,
  target_path TEXT NOT NULL,
  device_id TEXT NOT NULL REFERENCES lws_devices(id),
  PRIMARY KEY(application_id, service, target_path)
);
