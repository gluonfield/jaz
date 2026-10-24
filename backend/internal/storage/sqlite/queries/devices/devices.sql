-- name: ListDevices :many
SELECT
  id,
  name,
  kind,
  status,
  public_key,
  platform,
  device_family,
  model_identifier,
  token_hash,
  created_at_ms,
  approved_at_ms,
  revoked_at_ms,
  last_seen_at_ms,
  last_seen_ip,
  user_agent,
  app_version
FROM devices
ORDER BY
  CASE status
    WHEN 'pending' THEN 0
    WHEN 'approved' THEN 1
    ELSE 2
  END,
  last_seen_at_ms DESC,
  created_at_ms DESC;

-- name: CountApprovedDevices :one
SELECT count(*)
FROM devices
WHERE status = 'approved';

-- name: GetDevice :one
SELECT
  id,
  name,
  kind,
  status,
  public_key,
  platform,
  device_family,
  model_identifier,
  token_hash,
  created_at_ms,
  approved_at_ms,
  revoked_at_ms,
  last_seen_at_ms,
  last_seen_ip,
  user_agent,
  app_version
FROM devices
WHERE id = sqlc.arg(id)
LIMIT 1;

-- name: GetDeviceByTokenHash :one
SELECT
  id,
  name,
  kind,
  status,
  public_key,
  platform,
  device_family,
  model_identifier,
  token_hash,
  created_at_ms,
  approved_at_ms,
  revoked_at_ms,
  last_seen_at_ms,
  last_seen_ip,
  user_agent,
  app_version
FROM devices
WHERE token_hash = sqlc.arg(token_hash)
LIMIT 1;

-- name: SavePairingDevice :exec
-- The tagged placeholder is outside the bearer-token hash alphabet.
INSERT INTO devices (
  id,
  name,
  kind,
  status,
  public_key,
  platform,
  device_family,
  model_identifier,
  token_hash,
  created_at_ms,
  last_seen_ip,
  user_agent,
  app_version
) VALUES (
  sqlc.arg(id),
  sqlc.arg(name),
  sqlc.arg(kind),
  'pending',
  sqlc.arg(public_key),
  sqlc.arg(platform),
  sqlc.arg(device_family),
  sqlc.arg(model_identifier),
  'pending:' || sqlc.arg(id),
  sqlc.arg(created_at_ms),
  sqlc.arg(last_seen_ip),
  sqlc.arg(user_agent),
  sqlc.arg(app_version)
)
ON CONFLICT(id) DO NOTHING;

-- name: SaveApprovedDevice :exec
INSERT INTO devices (
  id,
  name,
  kind,
  status,
  public_key,
  platform,
  device_family,
  model_identifier,
  token_hash,
  created_at_ms,
  approved_at_ms,
  last_seen_at_ms,
  last_seen_ip,
  user_agent,
  app_version
) VALUES (
  sqlc.arg(id),
  sqlc.arg(name),
  sqlc.arg(kind),
  'approved',
  sqlc.arg(public_key),
  sqlc.arg(platform),
  sqlc.arg(device_family),
  sqlc.arg(model_identifier),
  sqlc.arg(token_hash),
  sqlc.arg(created_at_ms),
  sqlc.arg(approved_at_ms),
  sqlc.arg(last_seen_at_ms),
  sqlc.arg(last_seen_ip),
  sqlc.arg(user_agent),
  sqlc.arg(app_version)
)
ON CONFLICT(id) DO UPDATE SET
  name = excluded.name,
  kind = excluded.kind,
  status = 'approved',
  public_key = excluded.public_key,
  platform = excluded.platform,
  device_family = excluded.device_family,
  model_identifier = excluded.model_identifier,
  token_hash = excluded.token_hash,
  approved_at_ms = excluded.approved_at_ms,
  revoked_at_ms = 0,
  last_seen_at_ms = excluded.last_seen_at_ms,
  last_seen_ip = excluded.last_seen_ip,
  user_agent = excluded.user_agent,
  app_version = excluded.app_version;

-- name: UpdateDeviceSeen :execrows
UPDATE devices
SET
  last_seen_at_ms = sqlc.arg(last_seen_at_ms),
  last_seen_ip = sqlc.arg(last_seen_ip),
  user_agent = sqlc.arg(user_agent)
WHERE id = sqlc.arg(id);

-- name: RevokeDevice :execrows
UPDATE devices
SET
  status = 'revoked',
  revoked_at_ms = sqlc.arg(revoked_at_ms)
WHERE id = sqlc.arg(id)
  AND status != 'revoked';

-- name: CreatePairingRequest :exec
INSERT INTO device_pairing_requests (
  id, device_id, secret_hash, status, created_at_ms, expires_at_ms, profile_json
) VALUES (
  sqlc.arg(id), sqlc.arg(device_id), sqlc.arg(secret_hash), 'pending',
  sqlc.arg(created_at_ms), sqlc.arg(expires_at_ms), sqlc.arg(profile_json)
);

-- name: InvalidateDevicePairings :exec
UPDATE device_pairing_requests
SET status = 'rejected', secret_hash = '', rejected_at_ms = sqlc.arg(at_ms)
WHERE device_id = sqlc.arg(device_id) AND secret_hash != '';

-- name: GetPairingRequest :one
SELECT * FROM device_pairing_requests WHERE id = sqlc.arg(id);

-- name: ListPairingRequests :many
SELECT * FROM device_pairing_requests
WHERE created_at_ms >= sqlc.arg(after_ms)
ORDER BY created_at_ms DESC;

-- name: PairingAdmission :one
SELECT
  CAST(coalesce(sum(CASE WHEN created_at_ms > sqlc.arg(recent_after_ms) THEN 1 ELSE 0 END), 0) AS INTEGER) AS recent,
  CAST(coalesce(sum(CASE WHEN secret_hash != '' AND status IN ('pending', 'approved') AND expires_at_ms > sqlc.arg(at_ms) THEN 1 ELSE 0 END), 0) AS INTEGER) AS pending
FROM device_pairing_requests;

-- name: PrunePairingRequests :exec
DELETE FROM device_pairing_requests WHERE created_at_ms < sqlc.arg(before_ms);

-- name: PrunePairingDevices :exec
DELETE FROM devices
WHERE devices.token_hash = 'pending:' || devices.id
  AND devices.created_at_ms < sqlc.arg(before_ms)
  AND NOT EXISTS (SELECT 1 FROM device_pairing_requests WHERE device_id = devices.id);

-- name: ApprovePairingRequest :execrows
UPDATE device_pairing_requests
SET
  status = 'approved',
  approved_at_ms = sqlc.arg(approved_at_ms)
WHERE id = sqlc.arg(id)
  AND status = 'pending'
  AND expires_at_ms > sqlc.arg(approved_at_ms);

-- name: RejectPairingRequest :execrows
UPDATE device_pairing_requests
SET
  status = 'rejected',
  secret_hash = '',
  rejected_at_ms = sqlc.arg(rejected_at_ms)
WHERE id = sqlc.arg(id)
  AND status = 'pending';

-- name: ClaimPairingRequest :execrows
UPDATE device_pairing_requests
SET secret_hash = ''
WHERE device_pairing_requests.id = sqlc.arg(id)
  AND secret_hash = sqlc.arg(secret_hash)
  AND status = 'approved'
  AND expires_at_ms > sqlc.arg(at_ms);
