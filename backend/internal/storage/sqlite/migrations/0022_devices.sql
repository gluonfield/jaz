-- +goose Up
CREATE TABLE IF NOT EXISTS devices (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  kind TEXT NOT NULL,
  status TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  public_key TEXT NOT NULL DEFAULT '',
  created_at_ms INTEGER NOT NULL,
  approved_at_ms INTEGER NOT NULL DEFAULT 0,
  revoked_at_ms INTEGER NOT NULL DEFAULT 0,
  last_seen_at_ms INTEGER NOT NULL DEFAULT 0,
  last_seen_ip TEXT NOT NULL DEFAULT '',
  user_agent TEXT NOT NULL DEFAULT '',
  app_version TEXT NOT NULL DEFAULT '',
  platform TEXT NOT NULL DEFAULT '',
  device_family TEXT NOT NULL DEFAULT '',
  model_identifier TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_devices_status ON devices(status, last_seen_at_ms);

CREATE TABLE IF NOT EXISTS device_pairing_requests (
  id TEXT PRIMARY KEY,
  device_id TEXT NOT NULL,
  secret_hash TEXT NOT NULL,
  status TEXT NOT NULL,
  created_at_ms INTEGER NOT NULL,
  expires_at_ms INTEGER NOT NULL,
  approved_at_ms INTEGER NOT NULL DEFAULT 0,
  rejected_at_ms INTEGER NOT NULL DEFAULT 0,
  FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_device_pairing_requests_status ON device_pairing_requests(status, created_at_ms);
CREATE INDEX IF NOT EXISTS idx_device_pairing_requests_device ON device_pairing_requests(device_id);

-- +goose Down
DROP TABLE IF EXISTS device_pairing_requests;
DROP TABLE IF EXISTS devices;
