-- name: ListCloudAccounts :many
SELECT
  id,
  base_url,
  token_encrypted,
  oidc_login_url,
  user_id,
  org_id,
  org_name,
  email,
  display_name,
  role,
  capabilities_json,
  connected_at_ms,
  last_sync_at_ms,
  last_error
FROM cloud_accounts
ORDER BY connected_at_ms ASC, id ASC;

-- name: GetCloudAccount :one
SELECT
  id,
  base_url,
  token_encrypted,
  oidc_login_url,
  user_id,
  org_id,
  org_name,
  email,
  display_name,
  role,
  capabilities_json,
  connected_at_ms,
  last_sync_at_ms,
  last_error
FROM cloud_accounts
WHERE id = sqlc.arg(id);

-- name: UpsertCloudAccount :exec
INSERT INTO cloud_accounts (
  id,
  base_url,
  token_encrypted,
  oidc_login_url,
  user_id,
  org_id,
  org_name,
  email,
  display_name,
  role,
  capabilities_json,
  connected_at_ms,
  last_sync_at_ms,
  last_error
) VALUES (
  sqlc.arg(id),
  sqlc.arg(base_url),
  sqlc.arg(token_encrypted),
  sqlc.arg(oidc_login_url),
  sqlc.arg(user_id),
  sqlc.arg(org_id),
  sqlc.arg(org_name),
  sqlc.arg(email),
  sqlc.arg(display_name),
  sqlc.arg(role),
  sqlc.arg(capabilities_json),
  sqlc.arg(connected_at_ms),
  sqlc.arg(last_sync_at_ms),
  sqlc.arg(last_error)
)
ON CONFLICT(id) DO UPDATE SET
  base_url = excluded.base_url,
  token_encrypted = excluded.token_encrypted,
  oidc_login_url = excluded.oidc_login_url,
  user_id = excluded.user_id,
  org_id = excluded.org_id,
  org_name = excluded.org_name,
  email = excluded.email,
  display_name = excluded.display_name,
  role = excluded.role,
  capabilities_json = excluded.capabilities_json,
  connected_at_ms = excluded.connected_at_ms,
  last_sync_at_ms = excluded.last_sync_at_ms,
  last_error = excluded.last_error;

-- name: UpdateCloudAccountSyncState :exec
UPDATE cloud_accounts
SET last_sync_at_ms = sqlc.arg(last_sync_at_ms),
    last_error = sqlc.arg(last_error)
WHERE id = sqlc.arg(id);

-- name: UpdateCloudAccountToken :exec
UPDATE cloud_accounts
SET token_encrypted = sqlc.arg(token_encrypted)
WHERE id = sqlc.arg(id);

-- name: DeleteCloudAccount :exec
DELETE FROM cloud_accounts WHERE id = sqlc.arg(id);

-- name: ListCloudSpaces :many
SELECT account_id, id, org_id, name, created_by, created_at_ms, updated_at_ms
FROM cloud_spaces
WHERE account_id = sqlc.arg(account_id)
ORDER BY name ASC, id ASC;

-- name: UpsertCloudSpace :exec
INSERT INTO cloud_spaces (account_id, id, org_id, name, created_by, created_at_ms, updated_at_ms)
VALUES (
  sqlc.arg(account_id),
  sqlc.arg(id),
  sqlc.arg(org_id),
  sqlc.arg(name),
  sqlc.arg(created_by),
  sqlc.arg(created_at_ms),
  sqlc.arg(updated_at_ms)
)
ON CONFLICT(account_id, id) DO UPDATE SET
  org_id = excluded.org_id,
  name = excluded.name,
  created_by = excluded.created_by,
  updated_at_ms = excluded.updated_at_ms;

-- name: DeleteCloudSpacesByAccount :exec
DELETE FROM cloud_spaces WHERE account_id = sqlc.arg(account_id);

-- name: ListCloudThreads :many
SELECT
  account_id,
  id,
  org_id,
  owner_user_id,
  space_id,
  run_id,
  visibility,
  source_type,
  source_id,
  title,
  status,
  turn_epoch,
  model_profile_id,
  model,
  reasoning_effort,
  executor_target,
  runtime_ref_json,
  personal_sources_enabled,
  pinned,
  archived,
  unread,
  last_seq,
  created_at_ms,
  updated_at_ms
FROM cloud_threads
WHERE account_id = sqlc.arg(account_id)
  AND archived = sqlc.arg(archived)
ORDER BY updated_at_ms DESC, id ASC;

-- name: ListAllCloudThreadIDs :many
SELECT id FROM cloud_threads WHERE account_id = sqlc.arg(account_id);

-- name: GetCloudThread :one
SELECT
  account_id,
  id,
  org_id,
  owner_user_id,
  space_id,
  run_id,
  visibility,
  source_type,
  source_id,
  title,
  status,
  turn_epoch,
  model_profile_id,
  model,
  reasoning_effort,
  executor_target,
  runtime_ref_json,
  personal_sources_enabled,
  pinned,
  archived,
  unread,
  last_seq,
  created_at_ms,
  updated_at_ms
FROM cloud_threads
WHERE account_id = sqlc.arg(account_id)
  AND id = sqlc.arg(id);

-- name: UpsertCloudThread :exec
INSERT INTO cloud_threads (
  account_id,
  id,
  org_id,
  owner_user_id,
  space_id,
  run_id,
  visibility,
  source_type,
  source_id,
  title,
  status,
  turn_epoch,
  model_profile_id,
  model,
  reasoning_effort,
  executor_target,
  runtime_ref_json,
  personal_sources_enabled,
  pinned,
  archived,
  unread,
  last_seq,
  created_at_ms,
  updated_at_ms
) VALUES (
  sqlc.arg(account_id),
  sqlc.arg(id),
  sqlc.arg(org_id),
  sqlc.arg(owner_user_id),
  sqlc.arg(space_id),
  sqlc.arg(run_id),
  sqlc.arg(visibility),
  sqlc.arg(source_type),
  sqlc.arg(source_id),
  sqlc.arg(title),
  sqlc.arg(status),
  sqlc.arg(turn_epoch),
  sqlc.arg(model_profile_id),
  sqlc.arg(model),
  sqlc.arg(reasoning_effort),
  sqlc.arg(executor_target),
  sqlc.arg(runtime_ref_json),
  sqlc.arg(personal_sources_enabled),
  sqlc.arg(pinned),
  sqlc.arg(archived),
  sqlc.arg(unread),
  sqlc.arg(last_seq),
  sqlc.arg(created_at_ms),
  sqlc.arg(updated_at_ms)
)
ON CONFLICT(account_id, id) DO UPDATE SET
  org_id = excluded.org_id,
  owner_user_id = excluded.owner_user_id,
  space_id = excluded.space_id,
  run_id = excluded.run_id,
  visibility = excluded.visibility,
  source_type = excluded.source_type,
  source_id = excluded.source_id,
  title = excluded.title,
  status = excluded.status,
  turn_epoch = excluded.turn_epoch,
  model_profile_id = excluded.model_profile_id,
  model = excluded.model,
  reasoning_effort = excluded.reasoning_effort,
  executor_target = CASE WHEN excluded.executor_target != '' THEN excluded.executor_target ELSE cloud_threads.executor_target END,
  runtime_ref_json = excluded.runtime_ref_json,
  personal_sources_enabled = excluded.personal_sources_enabled,
  pinned = excluded.pinned,
  archived = excluded.archived,
  unread = excluded.unread,
  last_seq = MAX(cloud_threads.last_seq, excluded.last_seq),
  updated_at_ms = excluded.updated_at_ms;

-- name: UpdateCloudThreadUserState :exec
UPDATE cloud_threads
SET
  pinned = sqlc.arg(pinned),
  archived = sqlc.arg(archived),
  personal_sources_enabled = sqlc.arg(personal_sources_enabled),
  unread = sqlc.arg(unread)
WHERE account_id = sqlc.arg(account_id)
  AND id = sqlc.arg(id);

-- name: UpdateCloudThreadStatus :exec
UPDATE cloud_threads
SET status = sqlc.arg(status), updated_at_ms = sqlc.arg(updated_at_ms)
WHERE account_id = sqlc.arg(account_id)
  AND id = sqlc.arg(id);

-- name: UpdateCloudThreadLastSeq :exec
UPDATE cloud_threads
SET last_seq = MAX(last_seq, sqlc.arg(last_seq))
WHERE account_id = sqlc.arg(account_id)
  AND id = sqlc.arg(id);

-- name: DeleteCloudThread :exec
DELETE FROM cloud_threads
WHERE account_id = sqlc.arg(account_id)
  AND id = sqlc.arg(id);

-- name: DeleteCloudThreadsByAccount :exec
DELETE FROM cloud_threads WHERE account_id = sqlc.arg(account_id);

-- name: UpsertCloudThreadEvent :exec
INSERT INTO cloud_thread_events (
  account_id,
  thread_id,
  seq,
  org_id,
  turn_epoch,
  actor_user_id,
  type,
  payload_json,
  created_at_ms
) VALUES (
  sqlc.arg(account_id),
  sqlc.arg(thread_id),
  sqlc.arg(seq),
  sqlc.arg(org_id),
  sqlc.arg(turn_epoch),
  sqlc.arg(actor_user_id),
  sqlc.arg(type),
  sqlc.arg(payload_json),
  sqlc.arg(created_at_ms)
)
ON CONFLICT(account_id, thread_id, seq) DO UPDATE SET
  turn_epoch = excluded.turn_epoch,
  actor_user_id = excluded.actor_user_id,
  type = excluded.type,
  payload_json = excluded.payload_json,
  created_at_ms = excluded.created_at_ms;

-- name: ListCloudThreadEventsAfter :many
SELECT
  account_id,
  thread_id,
  seq,
  org_id,
  turn_epoch,
  actor_user_id,
  type,
  payload_json,
  created_at_ms
FROM cloud_thread_events
WHERE account_id = sqlc.arg(account_id)
  AND thread_id = sqlc.arg(thread_id)
  AND seq > sqlc.arg(after_seq)
ORDER BY seq ASC;

-- name: DeleteCloudThreadEventsByAccount :exec
DELETE FROM cloud_thread_events WHERE account_id = sqlc.arg(account_id);

-- name: GetCloudInstruction :one
SELECT account_id, org_id, scope, scope_id, kind, body_md, version, updated_by, updated_at_ms
FROM cloud_instructions
WHERE account_id = sqlc.arg(account_id)
  AND org_id = sqlc.arg(org_id)
  AND scope = sqlc.arg(scope)
  AND scope_id = sqlc.arg(scope_id)
  AND kind = sqlc.arg(kind);

-- name: UpsertCloudInstruction :exec
INSERT INTO cloud_instructions (
  account_id,
  org_id,
  scope,
  scope_id,
  kind,
  body_md,
  version,
  updated_by,
  updated_at_ms
) VALUES (
  sqlc.arg(account_id),
  sqlc.arg(org_id),
  sqlc.arg(scope),
  sqlc.arg(scope_id),
  sqlc.arg(kind),
  sqlc.arg(body_md),
  sqlc.arg(version),
  sqlc.arg(updated_by),
  sqlc.arg(updated_at_ms)
)
ON CONFLICT(account_id, org_id, scope, scope_id, kind) DO UPDATE SET
  body_md = excluded.body_md,
  version = excluded.version,
  updated_by = excluded.updated_by,
  updated_at_ms = excluded.updated_at_ms;

-- name: DeleteCloudInstructionsByAccount :exec
DELETE FROM cloud_instructions WHERE account_id = sqlc.arg(account_id);

-- name: UpsertCloudUsageEvent :exec
INSERT INTO cloud_usage_events (
  account_id,
  id,
  org_id,
  user_id,
  thread_id,
  model_profile_id,
  model,
  input_tokens,
  cached_input_tokens,
  output_tokens,
  cost_usd,
  created_at_ms
) VALUES (
  sqlc.arg(account_id),
  sqlc.arg(id),
  sqlc.arg(org_id),
  sqlc.arg(user_id),
  sqlc.arg(thread_id),
  sqlc.arg(model_profile_id),
  sqlc.arg(model),
  sqlc.arg(input_tokens),
  sqlc.arg(cached_input_tokens),
  sqlc.arg(output_tokens),
  sqlc.arg(cost_usd),
  sqlc.arg(created_at_ms)
)
ON CONFLICT(account_id, id) DO NOTHING;

-- name: MaxCloudUsageEventID :one
SELECT COALESCE(MAX(id), 0) AS max_id
FROM cloud_usage_events
WHERE account_id = sqlc.arg(account_id);

-- name: DeleteCloudUsageEventsByAccount :exec
DELETE FROM cloud_usage_events WHERE account_id = sqlc.arg(account_id);
