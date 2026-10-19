-- +goose Up
ALTER TABLE messages RENAME COLUMN from_agent TO from_user;
UPDATE messages SET from_user = CASE WHEN role = 'user' AND from_user = 0 THEN 1 ELSE 0 END;

-- +goose Down
UPDATE messages SET from_user = CASE WHEN role = 'user' AND from_user = 0 THEN 1 ELSE 0 END;
ALTER TABLE messages RENAME COLUMN from_user TO from_agent;
