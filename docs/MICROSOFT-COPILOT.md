# Microsoft Copilot Work/School

Jaz uses the official Microsoft 365 Copilot Chat API through its built-in ACP bridge. Users need an eligible Microsoft 365 Copilot work/school licence.

Configure Jaz's Microsoft Entra application once on the server:

1. Register an application for work/school accounts. Use a multitenant registration to support other organizations.
2. Set `JAZ_MICROSOFT_COPILOT_OAUTH_CLIENT_ID` to its application/client ID. For a confidential web registration, also set `JAZ_MICROSOFT_COPILOT_OAUTH_CLIENT_SECRET`; public clients use PKCE.
3. Register the redirect URI `<Jaz public URL>/v1/acp/agents/microsoft_copilot/auth/callback`. Local development defaults to `http://127.0.0.1:5299/v1/acp/agents/microsoft_copilot/auth/callback`.
4. Configure the delegated Graph permissions below and obtain tenant consent as required. Jaz requests `openid`, `profile` and `offline_access` with them.

Required Graph permissions: `Sites.Read.All`, `Mail.Read`, `People.Read.All`, `OnlineMeetingTranscript.Read.All`, `Chat.Read`, `ChannelMessage.Read.All`, `ExternalItem.Read.All`.

`JAZ_MICROSOFT_COPILOT_OAUTH_TENANT_ID` defaults to `organizations`; a specific work/school tenant ID or domain may be configured. `JAZ_MICROSOFT_COPILOT_OAUTH_REDIRECT_URL` overrides the callback URL. `JAZ_MICROSOFT_COPILOT_OAUTH_USE_BROKER=true` uses Jaz's configured OAuth relay.

Users then sign in from **Settings → Agents → Microsoft Copilot Work/School**. This beta API supports text and Microsoft 365 file URLs. Model identity, context limits and token usage remain unknown when Microsoft does not provide them.

[Microsoft Chat API documentation](https://learn.microsoft.com/en-us/microsoft-365/copilot/extensibility/api/ai-services/chat/overview)
