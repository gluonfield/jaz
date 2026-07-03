-- +goose Up
UPDATE threads
SET goal = '{}'
WHERE json_valid(goal)
  AND (json_extract(goal, '$.provider') IS NOT NULL
       OR json_extract(goal, '$.provider_goal_id') IS NOT NULL);

ALTER TABLE threads DROP COLUMN runtime_capabilities;

-- +goose Down
ALTER TABLE threads ADD COLUMN runtime_capabilities TEXT NOT NULL DEFAULT '{}';
