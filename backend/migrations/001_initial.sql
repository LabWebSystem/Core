CREATE TABLE IF NOT EXISTS applications (
  id TEXT PRIMARY KEY,
  subdomain TEXT NOT NULL UNIQUE,
  repository_url TEXT NOT NULL,
  git_ref TEXT NOT NULL,
  manifest_name TEXT NOT NULL DEFAULT '',
  manifest_description TEXT NOT NULL DEFAULT '',
  desired_state TEXT NOT NULL DEFAULT 'STOPPED',
  observed_state TEXT NOT NULL DEFAULT 'UNKNOWN',
  registration_state TEXT NOT NULL DEFAULT 'ACTIVE',
  revision TEXT NOT NULL DEFAULT '',
  latest_error TEXT NOT NULL DEFAULT '',
  installation_id TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS application_variables (
  application_id TEXT NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  value TEXT NOT NULL,
  is_secret INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (application_id, name)
);
CREATE TABLE IF NOT EXISTS operations (
  id TEXT PRIMARY KEY,
  application_id TEXT REFERENCES applications(id) ON DELETE CASCADE,
  request_id TEXT NOT NULL UNIQUE,
  kind TEXT NOT NULL,
  state TEXT NOT NULL,
  error_message TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS operations_application_state ON operations(application_id, state);
