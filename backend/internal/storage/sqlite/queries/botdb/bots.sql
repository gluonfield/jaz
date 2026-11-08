-- name: UpsertBot :exec
INSERT INTO bots (thread_id, kind, prompt, shape, color, members, main, workspaces)
VALUES (sqlc.arg(thread_id), sqlc.arg(kind), sqlc.arg(prompt), sqlc.arg(shape), sqlc.arg(color), sqlc.arg(members), sqlc.arg(main), sqlc.arg(workspaces))
ON CONFLICT(thread_id) DO UPDATE SET
  kind = excluded.kind,
  main = excluded.main,
  prompt = excluded.prompt,
  shape = excluded.shape,
  color = excluded.color,
  members = excluded.members,
  workspaces = excluded.workspaces;

-- name: GetBot :one
SELECT thread_id, kind, shape, color, members, pinned, prompt, main, workspaces
FROM bots
WHERE thread_id = sqlc.arg(thread_id)
LIMIT 1;

-- name: ListBots :many
SELECT thread_id, kind, shape, color, members, pinned, prompt, main, workspaces
FROM bots;

-- name: UnpinBots :exec
UPDATE bots SET pinned = 0;

-- name: PinBot :exec
UPDATE bots SET pinned = sqlc.arg(pinned) WHERE thread_id = sqlc.arg(thread_id);

-- name: SaveMembership :exec
INSERT INTO bot_memberships (group_id, bot_id, thread_id, seen)
VALUES (sqlc.arg(group_id), sqlc.arg(bot_id), sqlc.arg(thread_id), sqlc.arg(seen))
ON CONFLICT(group_id, bot_id) DO UPDATE SET
  thread_id = excluded.thread_id,
  seen = excluded.seen;

-- name: GetMembership :one
SELECT group_id, bot_id, thread_id, seen
FROM bot_memberships
WHERE group_id = sqlc.arg(group_id) AND bot_id = sqlc.arg(bot_id)
LIMIT 1;

-- name: GetMembershipByThread :one
SELECT group_id, bot_id, thread_id, seen
FROM bot_memberships
WHERE thread_id = sqlc.arg(thread_id)
LIMIT 1;

-- name: ListMemberships :many
SELECT group_id, bot_id, thread_id, seen
FROM bot_memberships;
