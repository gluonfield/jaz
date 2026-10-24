-- +goose Up
ALTER TABLE device_pairing_requests ADD COLUMN profile_json TEXT NOT NULL DEFAULT '{}';

UPDATE device_pairing_requests
SET profile_json = (
  SELECT json_object(
    'id', d.id, 'name', d.name, 'kind', d.kind, 'status', d.status,
    'public_key', d.public_key, 'platform', d.platform,
    'device_family', d.device_family, 'model_identifier', d.model_identifier,
    'last_seen_ip', d.last_seen_ip, 'user_agent', d.user_agent, 'app_version', d.app_version
  )
  FROM devices d WHERE d.id = device_pairing_requests.device_id
);

UPDATE device_pairing_requests
SET status = 'rejected', secret_hash = ''
WHERE secret_hash != '';

-- +goose Down
ALTER TABLE device_pairing_requests DROP COLUMN profile_json;
