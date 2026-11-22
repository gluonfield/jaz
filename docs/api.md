# Jaz REST API

Use the REST API to register project folders and configure bots from a script or an integration. These endpoints are served by the Jaz backend used by the desktop and web clients. This guide covers device enrollment, projects, and bots.

## Make a request

Use the origin of your Jaz backend and HTTPS for remote connections. Send an approved device token in the `Authorization: Bearer` header and use `Content-Type: application/json` for JSON request bodies. `X-Jaz-Client-Platform: cli` identifies a script or other command-line client and is optional.

This Node.js example lists saved projects. Supply the backend origin and device token through your client's configuration or secret store. The example reads them from environment variables that you set for the client.

```js
const { JAZ_API_URL, JAZ_DEVICE_TOKEN } = process.env
if (!JAZ_API_URL || !JAZ_DEVICE_TOKEN) {
  throw new Error('Set JAZ_API_URL and JAZ_DEVICE_TOKEN')
}

const response = await fetch(new URL('/v1/projects', JAZ_API_URL), {
  redirect: 'error',
  headers: { Authorization: `Bearer ${JAZ_DEVICE_TOKEN}` },
})
if (!response.ok) throw new Error(`Jaz returned HTTP ${response.status}`)
const { projects } = await response.json()
```

`GET /health` checks backend reachability. `GET /v1/auth/check` verifies the bearer and returns `auth_kind: "device"` for an approved device. Jaz does not inject a REST credential into ACP subprocesses.

## Enroll an API client

1. Generate a dedicated identity once and reuse it when enrolling the same client again. The backend expects 32 random bytes encoded as unpadded base64url in `public_key` and their SHA-256 digest encoded as lowercase hex in `device_id`.
2. Send `POST /v1/devices/pairing-requests` with `name`, `kind`, `device_id`, and `public_key` in the JSON body. The response is `202 Accepted` with `pairing.id`, `pairing.expires_at`, and `pairing_secret`.
3. Have the backend owner approve that request in **Settings → Devices → Pending approval** on the host computer. For a headless backend, see the [server-side approval instructions](remote-backend.md#device-cli). Poll `GET /v1/devices/pairing-requests/{id}` every few seconds, using the `X-Jaz-Pairing-Secret` header. Stop if the request is rejected or expires (`410 Gone`).
4. The first successful claim after approval returns the `token`. Save it in the client's secret store and use it as the bearer. If you lose it, enroll the client again.

For example, generate the enrollment body in Node.js:

```js
import { createHash, randomBytes } from 'node:crypto'

const identity = randomBytes(32)
const enrollment = {
  name: 'Example automation',
  kind: 'cli',
  device_id: createHash('sha256').update(identity).digest('hex'),
  public_key: identity.toString('base64url'),
}
```

A device token carries the usual access of an approved Jaz device, including access beyond the routes below. Use a separate identity for each integration so its access can be revoked independently in device settings. Keep device tokens and pairing secrets out of URLs, prompts, logs, and source control. Reject redirects on requests that carry them.

## Projects

Project paths must be absolute directories accessible to the backend. Clone or mount a repository before registering it. A project record contains `name`, `path`, and `git`.

| Method and path | Request | Success response |
| --- | --- | --- |
| `GET /v1/projects` | | `200` with a `projects` array |
| `POST /v1/projects` | `{ "path": "/workspace/example" }` | `200` with the project record |
| `DELETE /v1/projects?path=<encoded-path>` | URL-encode the absolute path | `200` with the remaining `projects` array |
| `PUT /v1/projects/order` | `{ "paths": ["/workspace/example"] }` | `200` with the reordered `projects` array |

Deleting a project removes its saved entry. Reordering moves the supplied saved paths to the front in that order and keeps the remaining paths in their previous order.

## Bots

| Method and path | Request | Success response |
| --- | --- | --- |
| `GET /v1/bots` | | `200` with a `bots` array |
| `POST /v1/bots` | A bot definition, as shown below | `200` with the created bot |
| `GET /v1/bots/{bot}` | Use the bot's `id` | `200` with the bot |
| `PATCH /v1/bots/{bot}` | Supply the fields to change, for example `{ "name": "Example assistant" }` | `200` with the updated bot |
| `DELETE /v1/bots/{bot}` | | `204 No Content` |
| `POST /v1/bots/{bot}/messages` | `{ "text": "Summarize the project README." }` | `202 Accepted` with an empty body. Processing is asynchronous. |
| `GET /v1/bots/{bot}/activity` | | `200` with the activity view |
| `GET /v1/bots/{bot}/files` | | `200` with a `files` array |
| `PUT /v1/bots/{bot}/files/{name}` | `content` and the `version` read from the file list | `200` with the saved file. A stale version returns `409` with the current file. |

Creating a bot requires a name and an enabled agent on the backend. The optional `agent` and `model` fields select a configured agent and model. `prompt` sets the bot's role, and `workspaces` links existing directories accessible to the backend. Register those directories as projects if you also want them in the project picker.

```json
{
  "name": "Example assistant",
  "prompt": "Maintain this project and follow its repository instructions.",
  "workspaces": ["/workspace/example"]
}
```

## Authentication errors

A `401` with code `unauthorized` means the bearer was missing or rejected. A `403` with code `device_approval_required` means the caller needs an approved device credential. Check the backend origin and the credential, then use the enrollment flow if the client needs a new approval.

For deployment and browser proxy settings, see [Remote Backend](remote-backend.md). For route definitions, see the [backend router](../backend/internal/server/routes.go) and [mounted API routes](../backend/internal/app/routes.go).
