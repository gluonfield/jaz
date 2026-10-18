-- +goose Up
-- The user's main bot, which Jaz creates once; the index allows only one.
ALTER TABLE bots ADD COLUMN main INTEGER NOT NULL DEFAULT 0;
CREATE UNIQUE INDEX bots_one_main ON bots(main) WHERE main = 1;

-- +goose Down
DROP INDEX bots_one_main;
ALTER TABLE bots DROP COLUMN main;
