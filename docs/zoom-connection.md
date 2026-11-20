# Zoom connection

Zoom is available in Connections and uses its official HTTP MCP endpoint, `https://mcp.zoom.us/mcp/zoom/streamable`.

Zoom requires manual OAuth client registration for custom MCP clients. Create a General app in Zoom App Marketplace with the scopes needed for the tools you plan to use.

## Local desktop

Open Connections on the device running the local Jaz backend. A remote Jaz client shows **Local Sign In Required** for a public client.

1. Enable **Use Public Client OAuth** in the Zoom app’s credentials. For a production client that requires HTTPS, register `https://jaz.chat/oauth/callback` as the public redirect URL and set `JAZ_ZOOM_OAUTH_PUBLIC_CLIENT_USE_BROKER=true` in the local Jaz backend’s environment. For a development client that accepts loopback, register `http://127.0.0.1:5299/v1/mcp/oauth/callback` and leave the broker option unset.
2. Set `JAZ_ZOOM_OAUTH_PUBLIC_CLIENT_ID` in the local Jaz backend’s environment to the matching public client ID. Restart Jaz, select Zoom → Connect, and approve Zoom’s consent screen.

Jaz uses S256 PKCE and an ephemeral local listener. With the broker enabled, the existing HTTPS callback relays the authorization code to that listener. The PKCE verifier, token exchange, and refresh stay on the local backend. Public OAuth requires no client secret.

## Hosted backend

Configure the app’s confidential OAuth client on the Jaz backend. An explicitly configured confidential client takes precedence over the public client.

- `JAZ_ZOOM_OAUTH_CLIENT_ID`: the app’s Client ID.
- `JAZ_ZOOM_OAUTH_CLIENT_SECRET`: the app’s Client Secret, in the backend process environment. To reference another environment variable, set `JAZ_ZOOM_OAUTH_CLIENT_SECRET_ENV_VAR` to its name.
- In the Zoom app’s OAuth settings, register the redirect URL `https://jaz.chat/oauth/callback`. If the Jaz backend sets `jaz.connections.oauthredirectbrokerurl`, register that callback URL instead.

Restart the Jaz backend, then select Zoom → Connect and approve Zoom’s consent screen. Connections shows **Setup Required** until the OAuth client is configured.

The equivalent YAML configuration uses `jaz.connections.mcp.zoom.publicclientid`, `publicclientusebroker`, `clientid`, `clientsecretenvvar` and `useappscopes` (default `true`). Jaz uses the app’s configured scopes unless the server’s OAuth challenge requests specific scopes.

[Zoom MCP setup](https://developers.zoom.us/docs/mcp/servers/connect-to-zoom-mcp-servers/) · [Zoom public-client OAuth](https://developers.zoom.us/docs/integrations/PKCE/)
