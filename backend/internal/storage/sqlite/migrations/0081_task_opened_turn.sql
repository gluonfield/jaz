-- +goose Up
-- When the bot turn that opened the task began; 0 outside a turn.
ALTER TABLE bot_commitments ADD COLUMN opened_turn_ms INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE bot_commitments DROP COLUMN opened_turn_ms;
