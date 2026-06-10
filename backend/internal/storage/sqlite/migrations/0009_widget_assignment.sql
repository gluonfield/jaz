-- +goose Up
-- Widget enablement is derived from board assignment now; the flag is gone.
ALTER TABLE loops DROP COLUMN widget_enabled;

-- +goose Down
ALTER TABLE loops ADD COLUMN widget_enabled INTEGER NOT NULL DEFAULT 0;
