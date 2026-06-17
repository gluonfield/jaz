-- name: ListDevices :many
SELECT
  id,
  name,
  kind,
  status,
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

-- name: CreateDevice :exec
INSERT INTO devices (
  id,
  name,
  kind,
  status,
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
  sqlc.arg(status),
  sqlc.arg(token_hash),
  sqlc.arg(created_at_ms),
  sqlc.arg(approved_at_ms),
  sqlc.arg(last_seen_at_ms),
  sqlc.arg(last_seen_ip),
  sqlc.arg(user_agent),
  sqlc.arg(app_version)
);

-- name: UpdateDeviceSeen :execrows
UPDATE devices
SET
  last_seen_at_ms = sqlc.arg(last_seen_at_ms),
  last_seen_ip = sqlc.arg(last_seen_ip),
  user_agent = sqlc.arg(user_agent)
WHERE id = sqlc.arg(id);

-- name: ApproveDevice :execrows
UPDATE devices
SET
  status = 'approved',
  approved_at_ms = sqlc.arg(approved_at_ms),
  revoked_at_ms = 0
WHERE id = sqlc.arg(id)
  AND status = 'pending';

-- name: RevokeDevice :execrows
UPDATE devices
SET
  status = 'revoked',
  revoked_at_ms = sqlc.arg(revoked_at_ms)
WHERE id = sqlc.arg(id)
  AND status != 'revoked';

-- name: RenameDevice :execrows
UPDATE devices
SET name = sqlc.arg(name)
WHERE id = sqlc.arg(id);

-- name: CreatePairingRequest :exec
INSERT INTO device_pairing_requests (
  id,
  device_id,
  secret_hash,
  status,
  created_at_ms,
  expires_at_ms
) VALUES (
  sqlc.arg(id),
  sqlc.arg(device_id),
  sqlc.arg(secret_hash),
  sqlc.arg(status),
  sqlc.arg(created_at_ms),
  sqlc.arg(expires_at_ms)
);

-- name: GetPairingRequest :one
SELECT
  p.id,
  p.device_id,
  p.secret_hash,
  p.status,
  p.created_at_ms,
  p.expires_at_ms,
  p.approved_at_ms,
  p.rejected_at_ms,
  d.id AS device_db_id,
  d.name AS device_name,
  d.kind AS device_kind,
  d.status AS device_status,
  d.token_hash AS device_token_hash,
  d.created_at_ms AS device_created_at_ms,
  d.approved_at_ms AS device_approved_at_ms,
  d.revoked_at_ms AS device_revoked_at_ms,
  d.last_seen_at_ms AS device_last_seen_at_ms,
  d.last_seen_ip AS device_last_seen_ip,
  d.user_agent AS device_user_agent,
  d.app_version AS device_app_version
FROM device_pairing_requests p
JOIN devices d ON d.id = p.device_id
WHERE p.id = sqlc.arg(id)
LIMIT 1;

-- name: ListPairingRequests :many
SELECT
  p.id,
  p.device_id,
  p.secret_hash,
  p.status,
  p.created_at_ms,
  p.expires_at_ms,
  p.approved_at_ms,
  p.rejected_at_ms,
  d.id AS device_db_id,
  d.name AS device_name,
  d.kind AS device_kind,
  d.status AS device_status,
  d.token_hash AS device_token_hash,
  d.created_at_ms AS device_created_at_ms,
  d.approved_at_ms AS device_approved_at_ms,
  d.revoked_at_ms AS device_revoked_at_ms,
  d.last_seen_at_ms AS device_last_seen_at_ms,
  d.last_seen_ip AS device_last_seen_ip,
  d.user_agent AS device_user_agent,
  d.app_version AS device_app_version
FROM device_pairing_requests p
JOIN devices d ON d.id = p.device_id
ORDER BY p.created_at_ms DESC;

-- name: ApprovePairingRequest :execrows
UPDATE device_pairing_requests
SET
  status = 'approved',
  approved_at_ms = sqlc.arg(approved_at_ms)
WHERE id = sqlc.arg(id)
  AND status = 'pending';

-- name: RejectPairingRequest :execrows
UPDATE device_pairing_requests
SET
  status = 'rejected',
  rejected_at_ms = sqlc.arg(rejected_at_ms)
WHERE id = sqlc.arg(id)
  AND status = 'pending';

-- name: ExpirePairingRequest :execrows
UPDATE device_pairing_requests
SET
  status = 'expired',
  rejected_at_ms = sqlc.arg(rejected_at_ms)
WHERE id = sqlc.arg(id)
  AND status = 'pending';

