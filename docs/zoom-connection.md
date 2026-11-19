# Zoom connection

Zoom is available in Connections and uses its official HTTP MCP endpoint, `https://mcp.zoom.us/mcp/zoom/streamable`.

Configure the Jaz backend with a pre-registered Zoom Marketplace General app:

- `JAZ_ZOOM_OAUTH_CLIENT_ID`: the app’s Client ID.
- `JAZ_ZOOM_OAUTH_CLIENT_SECRET`: the app’s Client Secret, in the backend process environment. To reference another environment variable, set `JAZ_ZOOM_OAUTH_CLIENT_SECRET_ENV_VAR` to its name.
- Register the callback shown in Settings → MCP → Edit → Advanced → OAuth, normally `https://jaz.chat/oauth/callback`. A custom deployment broker changes this URL.
- Enable only the Zoom app scopes needed for the intended tools. Jaz omits the consent `scope` parameter for this managed client, so Zoom uses the app’s configured scope defaults. An explicit server challenge requesting scopes still takes precedence.

Then select Zoom → Connect and approve Zoom’s consent screen. The connection appears only after successful authorization and the MCP tools use the stored user grant, including refresh. Missing deployment credentials produce an actionable setup error.

The equivalent YAML configuration uses `jaz.connections.mcp.zoom.clientid`, `clientsecretenvvar` and `useappscopes` (default `true`). Secret values are never part of the catalogue or connection metadata.

[Zoom MCP setup](https://developers.zoom.us/docs/mcp/) · [Zoom OAuth scopes](https://developers.zoom.us/docs/integrations/oauth-scopes-overview/)
