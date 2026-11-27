-- +goose Up
ALTER TABLE threads ADD COLUMN cloud_credential_profile_id TEXT;

-- +goose Down
ALTER TABLE threads DROP COLUMN cloud_credential_profile_id;
