# Zoom connection

Zoom is available in Connections and uses its official HTTP MCP endpoint, `https://mcp.zoom.us/mcp/zoom/streamable`.

Zoom requires manual OAuth client registration for custom MCP clients. Create a General app in Zoom App Marketplace, then configure its credentials on the Jaz backend.

- `JAZ_ZOOM_OAUTH_CLIENT_ID`: the app’s Client ID.
- `JAZ_ZOOM_OAUTH_CLIENT_SECRET`: the app’s Client Secret, in the backend process environment. To reference another environment variable, set `JAZ_ZOOM_OAUTH_CLIENT_SECRET_ENV_VAR` to its name.
- In the Zoom app’s OAuth settings, register the redirect URL `https://jaz.chat/oauth/callback`. If the Jaz backend sets `jaz.connections.oauthredirectbrokerurl`, register that callback URL instead.
- Enable only the Zoom app scopes needed for the intended tools. Jaz omits the consent `scope` parameter for this managed client, so Zoom uses the app’s configured scope defaults. An explicit server challenge requesting scopes still takes precedence.

Restart the Jaz backend, then select Zoom → Connect and approve Zoom’s consent screen. Connections shows **Setup Required** while either credential is missing.

The equivalent YAML configuration uses `jaz.connections.mcp.zoom.clientid`, `clientsecretenvvar` and `useappscopes` (default `true`). Secret values are never part of the catalogue or connection metadata.

[Zoom MCP setup](https://developers.zoom.us/docs/mcp/servers/connect-to-zoom-mcp-servers/) · [Zoom OAuth scopes](https://developers.zoom.us/docs/integrations/oauth-scopes-overview/)
