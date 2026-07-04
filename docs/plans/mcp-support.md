# MCP Support Implementation Plan

## Goal

Add remote MCP support to flightrecorder so a local agent can connect to a deployed flightrecorder API, complete a browser-based authorization flow, and receive project-scoped access to telemetry analysis and project configuration tools.

The agent authorization model must be separate from both browser admin sessions and telemetry reporter tokens. Agents may add projects, update project configuration, and query project analytics. Agents must not manage admin users, invitations, telemetry tokens, or agent tokens.

## Protocol Baseline

Target the latest MCP specification available while this plan was written: `2025-11-25`.

Relevant protocol expectations:

- MCP remote servers should use Streamable HTTP, a single endpoint supporting JSON-RPC over HTTP `POST`, with optional `GET` SSE support.
- MCP tools are exposed through `tools/list` and invoked through `tools/call`.
- MCP resources can expose contextual data through `resources/list`, `resources/read`, and resource templates.
- HTTP MCP authorization should follow the MCP OAuth profile: protected resource metadata, authorization server metadata, bearer tokens, PKCE, and a `resource` parameter that identifies the MCP server audience.

References:

- https://modelcontextprotocol.io/specification/2025-11-25
- https://modelcontextprotocol.io/specification/2025-11-25/basic/transports
- https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization
- https://modelcontextprotocol.io/specification/2025-11-25/server/tools
- https://modelcontextprotocol.io/specification/2025-11-25/server/resources

## Design Summary

Implement flightrecorder as both:

- An MCP resource server at `/mcp`, protected by agent bearer tokens.
- A small OAuth authorization server for MCP clients, reusing the existing admin browser login as the human identity check.

The browser flow:

1. A local MCP client starts without a token and calls `https://telemetry.example.com/mcp`.
2. flightrecorder returns `401` with `WWW-Authenticate: Bearer` and a protected resource metadata URL.
3. The MCP client discovers authorization metadata and opens the authorization URL in the user's browser, or prints the URL for copy/paste.
4. If the user is not logged in to the flightrecorder admin UI, the existing Google/dev login flow runs first.
5. The user lands on an agent consent screen.
6. The consent screen shows the requesting client name and:
   - A label: `Which project is this agent allowed to access?`
   - One checkbox per active project.
   - An `All Projects` checkbox.
   - A disabled `Confirm` button until at least one project or `All Projects` is selected.
7. On confirm, flightrecorder creates an agent authorization record and issues an OAuth authorization code.
8. The MCP client exchanges the code and PKCE verifier for an access token.
9. flightrecorder returns a token prefixed with `fr_agnt_`.
10. Subsequent MCP requests use `Authorization: Bearer fr_agnt_...`.

Telemetry ingest tokens should also change from the current `fr_` prefix to `fr_tel_`. Since flightrecorder is not yet in production, this can be done as a breaking token-format change without migration or backward compatibility.

## Non-Goals

- Do not let MCP clients manage admin users, invitations, telemetry ingest tokens, or agent authorizations.
- Do not expose raw SQL execution.
- Do not expose arbitrary database tables.
- Do not add MCP sampling, elicitation, or client-side roots in v1.
- Do not implement long-lived MCP server-to-client notifications in v1 unless needed by an SDK. Plain JSON responses are enough for all planned tools.

## Data Model

Add a migration for agent authorization tables.

```sql
CREATE TABLE agent_authorizations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id text NOT NULL,
    client_name text NOT NULL DEFAULT '',
    token_hash text,
    created_by_admin_user_id uuid REFERENCES admin_users(id) ON DELETE SET NULL,
    all_projects boolean NOT NULL DEFAULT false,
    scopes text[] NOT NULL DEFAULT ARRAY['mcp:read', 'mcp:write']::text[],
    enabled boolean NOT NULL DEFAULT true,
    expires_at timestamptz NOT NULL,
    activated_at timestamptz,
    last_used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX agent_authorizations_token_hash_idx
ON agent_authorizations(token_hash)
WHERE token_hash IS NOT NULL;

CREATE TABLE agent_authorization_projects (
    agent_authorization_id uuid NOT NULL REFERENCES agent_authorizations(id) ON DELETE CASCADE,
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    PRIMARY KEY (agent_authorization_id, project_id)
);

CREATE TABLE mcp_oauth_clients (
    client_id text PRIMARY KEY,
    client_name text NOT NULL,
    redirect_uris text[] NOT NULL,
    client_uri text NOT NULL DEFAULT '',
    logo_uri text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE mcp_oauth_codes (
    code_hash text PRIMARY KEY,
    client_id text NOT NULL,
    redirect_uri text NOT NULL,
    code_challenge text NOT NULL,
    code_challenge_method text NOT NULL,
    resource text NOT NULL,
    scopes text[] NOT NULL,
    admin_user_id uuid NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE,
    agent_authorization_id uuid NOT NULL REFERENCES agent_authorizations(id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
```

Notes:

- `agent_authorizations.token_hash` stores only the SHA-256 hash of the issued `fr_agnt_` token. It is null between consent and token exchange.
- `agent_authorization_projects` is empty when `all_projects = true`.
- `scopes` should exist even if v1 always grants `mcp:read mcp:write`; this gives us a place to add read-only agents later.
- `mcp_oauth_clients` is needed if we support dynamic registration or client metadata document caching.
- `mcp_oauth_codes` should be short lived, around 5 minutes.
- `agent_authorizations.expires_at` is non-null. The default v1 lifetime is 90 days, set when the authorization row is created.
- Add a matching `.down.sql` migration. Every existing schema migration has an up/down pair, and this one should drop indexes before tables in dependency order.
- Add a cleanup query for expired unconsumed OAuth codes and never-activated `agent_authorizations` where `token_hash IS NULL` and the linked code is expired.

## Token Prefixes

Update token generation helpers:

- Telemetry tokens: `fr_tel_<base64url-32-random-bytes>`
- Agent tokens: `fr_agnt_<base64url-32-random-bytes>`
- Invitations: keep `fr_invite_...`

Validation should be type-specific before hashing:

- Telemetry auth rejects anything not prefixed `fr_tel_`.
- MCP auth rejects anything not prefixed `fr_agnt_`.

This prevents token class confusion and makes debugging easier. Because there are no production tokens to rotate, no legacy `fr_` fallback is needed.

## OAuth and Browser Consent

### Endpoints

Protected resource metadata:

- `GET /.well-known/oauth-protected-resource`
- `GET /.well-known/oauth-protected-resource/mcp`

Authorization server metadata:

- `GET /.well-known/oauth-authorization-server`
- `GET /.well-known/openid-configuration` as an alias if easy; MCP clients must support both discovery mechanisms, and supporting both server-side improves compatibility.

If serving `/.well-known/openid-configuration`, keep it to OAuth-compatible metadata needed by MCP clients. Do not advertise OIDC-only capabilities such as `id_token` issuance unless flightrecorder actually implements them.

MCP OAuth endpoints:

- `GET /api/mcp/oauth/authorize`
- `POST /api/mcp/oauth/token`
- `POST /api/mcp/oauth/register` if implementing dynamic client registration.
- `GET /api/mcp/oauth/consent`
- `POST /api/mcp/oauth/consent`

MCP endpoint:

- `POST /mcp`
- `GET /mcp` returns `405` in v1 unless SSE is needed.

### Client Registration

The 2025-11-25 MCP auth spec prefers client ID metadata documents, allows preregistration, and keeps dynamic client registration as a fallback.

For v1:

1. Support public OAuth clients with PKCE and no client secret.
2. Accept `client_id` values that are HTTPS URLs and fetch/validate their client metadata document.
3. Also implement dynamic client registration if the Go OAuth surface remains small enough. This keeps compatibility with older MCP clients.
4. Store registered/discovered client metadata in `mcp_oauth_clients`.

Client metadata document fetching and validation must include:

- Require `https` client metadata URLs with a non-empty path.
- Resolve DNS and reject loopback, private, link-local, multicast, and otherwise non-public IP ranges.
- Cap redirects, and apply the same IP checks after each redirect.
- Use short request timeouts.
- Cap response body size.
- Require a JSON content type or reject non-JSON responses before parsing.
- Require `client_id`, `client_name`, and `redirect_uris` in the fetched metadata.
- Require fetched metadata `client_id` to exactly match the requested `client_id` URL.
- Require the requested `redirect_uri` to exactly match one of the metadata `redirect_uris`.

Redirect URI validation:

- Permit exact registered HTTPS redirect URIs.
- Permit loopback redirect URIs using `http://127.0.0.1:<port>/...` or `http://localhost:<port>/...` for local agents.
- Reject wildcard redirects.
- Reject query-string token delivery.

### Authorization Request

`GET /api/mcp/oauth/authorize` validates:

- `response_type=code`
- `client_id`
- `redirect_uri`
- `code_challenge`
- `code_challenge_method=S256`
- `state`
- `resource` equals the canonical MCP resource URL for this deployment, usually `${API_BASE_URL}/mcp`
- `scope` is empty or a subset of supported scopes

If the user lacks a valid admin session, redirect to the existing login route with a safe `return_path`.

If the user is logged in, redirect to the consent UI.

The validated authorization request must be stored server-side, or encoded in a signed short-lived consent token, before the browser reaches the consent page. The consent POST must reference only that opaque pending authorization ID/token and the selected projects. It must not trust client-supplied `client_id`, `redirect_uri`, `resource`, `scope`, or `code_challenge` fields, otherwise a malicious page could forge a consent POST against the user's admin cookie.

### Consent UI

Implement as a SPA page, backed by admin-cookie authenticated API endpoints.

Page behavior:

- Display requesting client name, client URI if present, and the target flightrecorder host.
- Display the label `Which project is this agent allowed to access?`
- Display an `All Projects` checkbox.
- Display one checkbox per project returned by `AdminListProjects`.
- Disable project checkboxes while `All Projects` is checked, or uncheck `All Projects` when a project is selected. Either interaction is acceptable, but it must be obvious.
- Disable `Confirm` until at least one project or `All Projects` is selected.
- Include `Cancel`, which redirects back to `redirect_uri` with `error=access_denied` and original `state`.

The consent POST creates:

- `agent_authorizations` with `enabled = true`.
- Zero or more `agent_authorization_projects`.
- One `mcp_oauth_codes` row bound to that authorization.

If the granting admin user is later disabled, agent authorizations created by that user should be suspended or rejected during validation. `created_by_admin_user_id` is nullable because of `ON DELETE SET NULL`; null creator means reject the agent token unless a future migration introduces an explicit system-owned authorization type.

The final redirect is:

```text
{redirect_uri}?code=...&state=...
```

The raw `fr_agnt_` token is not shown in the browser in the standards-compliant path. It is only returned from the token endpoint to the MCP client.

### Manual Link Fallback

Some agents may not be able to receive a loopback redirect. Add a non-advertised manual fallback only if needed after testing target clients:

- The agent prints a Flightrecorder authorization URL.
- The browser flow creates the same `agent_authorizations` row.
- The success page shows the `fr_agnt_` token exactly once for copy/paste.

This is less ideal than OAuth code exchange because it exposes the access token to the browser. Keep it off by default until we prove a client needs it.

### Token Endpoint

`POST /api/mcp/oauth/token` supports OAuth form-encoded requests with `Content-Type: application/x-www-form-urlencoded` and `grant_type=authorization_code`. Do not use the shared strict JSON body decoder for this endpoint.

- Validate authorization code hash.
- Validate not expired and not consumed.
- Validate exact `client_id`, `redirect_uri`, and `resource`.
- Validate PKCE S256 verifier.
- Mark code consumed.
- Generate the `fr_agnt_` token and update `agent_authorizations.token_hash` and `activated_at`.

Prefer generating the token at exchange time so an unredeemed code never has a live bearer token behind it.

Code consumption and token activation must run in one transaction. Use a consume-if-unused-and-unexpired update for the OAuth code, then update `agent_authorizations.token_hash` and `activated_at` before commit. If token activation fails, the code consumption must roll back so the client can retry.

Response:

```json
{
  "access_token": "fr_agnt_...",
  "token_type": "Bearer",
  "expires_in": 7776000,
  "scope": "mcp:read mcp:write"
}
```

Default lifetime: 90 days. Store `expires_at` on `agent_authorizations`. Make this configurable later if needed.

Refresh tokens are intentionally out of scope for v1.

## Agent Authorization Enforcement

Add `services.AgentAuth`:

```go
type AgentAuth interface {
    ValidateAgentToken(ctx context.Context, token string, resource string) (AgentSession, error)
}

type AgentSession struct {
    AuthorizationID uuid.UUID
    AdminUserID     *uuid.UUID
    ClientID        string
    ClientName      string
    AllProjects     bool
    ProjectIDs      map[uuid.UUID]bool
    ProjectKeys     map[string]uuid.UUID
    Scopes          map[string]bool
}
```

Validation should:

- Require `fr_agnt_` prefix.
- Hash token and load enabled authorization.
- Check expiry.
- Reject authorizations whose creator admin user is disabled.
- Reject authorizations with a null creator.
- Update `last_used_at`.
- Load accessible projects.
- Return 401 for invalid or expired token.
- Return 403 for valid token with insufficient scope or project access.

Project checks:

- If `all_projects`, allow every current project, including projects created after authorization.
- Otherwise, allow only projects linked at consent time.
- For `project_create`, require `all_projects = true`. This avoids the awkward case where a scoped token creates a project it was never explicitly authorized to access.
- For `project_update` and all query tools, require access to the target project.

Tool inputs should use the human-readable project key and name the field `project_key`. The admin API and `services.Admin` already use project keys for user-facing routes, so MCP should not expose internal UUIDs except in structured metadata where useful.

This matches the consent UI: users can authorize broad admin-like project access with `All Projects`, or deliberately constrain an agent to selected projects.

## MCP Server Surface

Add a new package, likely `api/mcp` plus `services/mcp.go`, rather than mixing JSON-RPC dispatch into `admin_handlers.go`.

### MCP initialize

Handle:

- `initialize`
- `notifications/initialized`
- `ping`

Server capabilities:

```json
{
  "tools": {},
  "resources": {}
}
```

Do not advertise `sampling`, `elicitation`, or subscriptions. Resource templates are served through `resources/templates/list`; they are not a separate top-level server capability.

### Tools

Use a small, explicit tool set. Names should be stable and boring.

Project management:

- `flightrecorder.projects.list`
- `flightrecorder.projects.create`
- `flightrecorder.projects.get_settings`
- `flightrecorder.projects.update`

Analytics:

- `flightrecorder.metrics.summary`
- `flightrecorder.events.list`
- `flightrecorder.events.types`
- `flightrecorder.players.trace`
- `flightrecorder.heatmap.regions`
- `flightrecorder.heatmap.zones`
- `flightrecorder.funnels.list`
- `flightrecorder.reports.list`
- `flightrecorder.reports.get`

Do not expose:

- `users.*`
- `invitations.*`
- `settings.ingest_tokens.*`
- `agent_authorizations.*`
- `reports.update` in v1. The requirement says agents can query feedback reports, not modify report triage state.

### Tool Schemas

All tools use JSON Schema input with `additionalProperties: false`.

Common filter shape for analytics tools:

```json
{
  "type": "object",
  "properties": {
    "project_key": { "type": "string" },
    "from": { "type": "string", "format": "date-time" },
    "to": { "type": "string", "format": "date-time" },
    "event_type": { "type": "string" },
    "region_id": { "type": "string" },
    "zone_id": { "type": "string" },
    "player_id": { "type": "string" },
    "game_version": { "type": "string" },
    "build_channel": { "type": "string" },
    "field_key": { "type": "string" },
    "field_value": {},
    "status": { "type": "string" },
    "label": { "type": "string" },
    "limit": { "type": "integer", "minimum": 1, "maximum": 500 }
  },
  "required": ["project_key"],
  "additionalProperties": false
}
```

Keep v1 limits conservative:

- Default limit: 100.
- Max limit: 500.
- Default time window: match the admin UI default, currently last 30 days.

### Tool Results

Return both:

- MCP `content` with concise human-readable text.
- `structuredContent` with the JSON result payload.

This lets agents reason over structured data without scraping text.

Example:

```json
{
  "content": [
    {
      "type": "text",
      "text": "Found 12 reports for project my-game."
    }
  ],
  "structuredContent": {
    "reports": []
  }
}
```

### Resources

Resources are useful for configuration context that agents may want to attach without executing a write tool.

Expose:

- `flightrecorder://projects` - list authorized projects.
- `flightrecorder://projects/{project_key}/settings` - project settings, schema, funnels, event groups.
- `flightrecorder://projects/{project_key}/query-fields` - query field definitions.
- `flightrecorder://projects/{project_key}/funnels` - configured funnel definitions.

Resource reads must enforce the same project access checks as tools.

## Service Layer Reuse

Reuse `services.Admin` methods where possible:

- `ListProjects`
- `CreateProject`
- `Settings`
- `Summary`
- `ListEvents`
- `PlayerTrace`
- `RegionHeatmap`
- `ZoneHeatmap`
- `Funnels`
- `ListReports`
- `GetReport`
- `EventTypes`

The current code has only an upsert path: the frontend uses `POST /projects` for both create and edit, and `adminService.CreateProject` ultimately calls `AdminUpsertProject`. MCP must not wrap that upsert blindly.

Required service split:

- Add an explicit create path used by `flightrecorder.projects.create`.
- Add an explicit update path used by `flightrecorder.projects.update`.
- The update path must first load the existing project by key, perform the agent project-scope check against that existing row, and only then call the shared normalization/upsert write.
- If the project key does not exist, `projects.update` must return 403 or 404 and must not create a row.
- `projects.create` remains allowed only for `All Projects` agents.

MCP `projects.update` should accept the same typed project config model as the admin API and rely on existing validation after the existence and scope checks.

Avoid adding MCP-specific business logic to SQL where the admin service already handles validation.

## Route Layout

In `api.Run`:

```go
mcpAuth := auth.RequireMCPAgent(opts.AgentAuth, log, opts.APIBaseURL + "/mcp")
h.POST("/mcp", mcpAuth, mcpHandler.HandlePost)
h.GET("/mcp", mcpAuth, mcpHandler.HandleGet)

h.GET("/.well-known/oauth-protected-resource", makeMCPProtectedResourceMetadata(...))
h.GET("/.well-known/oauth-protected-resource/mcp", makeMCPProtectedResourceMetadata(...))
h.GET("/.well-known/oauth-authorization-server", makeMCPAuthorizationServerMetadata(...))
h.GET("/.well-known/openid-configuration", makeMCPAuthorizationServerMetadata(...))

mcpOAuth := h.Group("/api/mcp/oauth")
registerMCPOAuthRoutes(mcpOAuth, ...)
```

Use separate middleware from:

- `auth.RequireAuth` for telemetry reporter tokens.
- `auth.RequireAdmin` for browser admin sessions.

The MCP handler should validate or at least tolerate the `MCP-Protocol-Version` header according to the Streamable HTTP transport. After `initialize`, clients are expected to send it on subsequent requests.

The `.well-known` routes need to be reachable at the origin root. Because Hertz currently uses `server.WithBasePath(cfg.APIBasePath)`, MCP support effectively requires `API_BASE_PATH=/` unless the server is changed to mount discovery routes outside the base path.

## Admin UI Changes

Add:

- A consent route for MCP authorization.
- An agent authorization management panel under Settings or Users.

The management panel should list:

- Agent/client name.
- Created by admin email.
- Access: `All Projects` or selected project names.
- Enabled.
- Expires.
- Last used.
- Created.

Allowed management actions for human admins:

- Revoke/disable an agent authorization.
- Delete expired/disabled authorizations if desired.

Do not allow agents to call these management actions.

## SQL Queries

Add sqlc queries for:

OAuth clients:

- `MCPUpsertOAuthClient`
- `MCPGetOAuthClient`

OAuth codes:

- `MCPCreateOAuthCode`
- `MCPConsumeOAuthCode`

Agent authorizations:

- `MCPCreateAgentAuthorization`
- `MCPCreateAgentAuthorizationProject`
- `MCPValidateAgentToken`
- `MCPListAgentAuthorizationProjects`
- `AdminListAgentAuthorizations`
- `AdminSetAgentAuthorizationEnabled`

Validation should use one indexed update where possible:

```sql
UPDATE agent_authorizations
SET last_used_at = now()
WHERE token_hash = $1
  AND enabled = true
  AND expires_at > now()
  AND created_by_admin_user_id IS NOT NULL
RETURNING id, created_by_admin_user_id, client_id, client_name, all_projects, scopes;
```

Then load the creator admin user, reject if disabled, and load scoped projects if `all_projects = false`.

## Security Notes

- Require HTTPS for deployed OAuth endpoints.
- Allow `http://localhost` and `http://127.0.0.1` redirect URIs for local agents.
- Always require PKCE S256 for public clients.
- Bind authorization codes and tokens to the MCP resource URL.
- Reject bearer tokens in query strings.
- Never log raw `fr_tel_` or `fr_agnt_` tokens.
- Store token hashes only.
- Add `WWW-Authenticate` challenges for 401 and insufficient-scope 403 responses.
- Reconcile `/mcp` Origin validation with the existing global CORS middleware. Non-browser MCP clients usually send no `Origin`; accept no-origin requests, and reject unexpected browser origins before processing JSON-RPC.
- Keep the `All Projects` grant visibly distinct in consent and management UI.
- Do not include screenshots or large payload blobs by default in report tools; expose screenshot URLs only if the caller asks for report detail.

## Implementation Steps

1. Add DB migration and sqlc queries.
   - Add `agent_authorizations`.
   - Add `agent_authorization_projects`.
   - Add `mcp_oauth_clients`.
   - Add `mcp_oauth_codes`.
   - Add the matching down migration.
   - Add cleanup for expired codes and never-activated authorizations.
   - Run `make sqlc`.

2. Split token helpers.
   - Change telemetry token generation to `fr_tel_`.
   - Add `NewAgentToken()` returning `fr_agnt_`.
   - Add prefix checks to telemetry and MCP auth validation.
   - Update docs and tests that mention ingest token prefixes.

3. Add `services.AgentAuth` and `services.MCPOAuth`.
   - Token validation.
   - OAuth client metadata validation/registration.
   - Authorization code creation and exchange.
   - Consent confirmation.

4. Add MCP OAuth HTTP routes.
   - Protected resource metadata.
   - Authorization server metadata.
   - Dynamic registration if included in v1.
   - Authorize, consent, token.
   - Redirect to existing admin login when needed.

5. Add consent UI.
   - New SPA route.
   - Project checkbox list plus `All Projects`.
   - Disabled confirm until selection.
   - Cancel flow.

6. Add MCP JSON-RPC handler.
   - Parse one JSON-RPC message per `POST`.
   - Require `POST` requests to include an `Accept` header supporting both `application/json` and `text/event-stream`.
   - Implement `initialize`, `ping`, `tools/list`, `tools/call`, `resources/list`, `resources/read`, `resources/templates/list`.
   - Return JSON-RPC errors with stable codes/messages.
   - Return `202` for notifications.

7. Implement MCP tools by wrapping `services.Admin`.
   - Add project access checks before calling the admin service.
   - Add scope checks for read vs write.
   - Preserve existing validation for project creation/update.
   - Add a regression test: scoped agent calls `projects.update` with an unknown project key and no project row is created.

8. Add agent authorization management UI for admins.
   - List authorizations.
   - Disable/revoke authorization.
   - No token display after creation.

9. Add documentation.
   - `docs/mcp.md` for users connecting local agents.
   - Update README feature list and auth section.
   - Document token prefixes.

10. Verify end to end.
   - Unit tests for OAuth validation, PKCE, code consumption, token prefixing, project scope enforcement.
   - API tests for metadata endpoints and 401/403 challenges.
   - MCP handler tests for initialize, tools/list, and representative tool calls.
   - Frontend build for consent and management UI.
   - Manual test with a local MCP client against local API.

## Test Plan

Backend:

- `AgentAuth` rejects telemetry tokens.
- Telemetry auth rejects agent tokens.
- Disabled agent authorization returns 401.
- Expired agent authorization returns 401.
- Agent authorization with a deleted/null creator returns 401.
- Agent authorization created by a disabled admin returns 401.
- Scoped agent cannot access unselected project.
- Scoped agent cannot create a project.
- Scoped agent calling `projects.update` with an unknown project key returns 403/404 and does not create a project.
- All-project agent can create a project.
- Agent tools cannot call user/token/invitation operations because those tools are not registered.
- OAuth authorize rejects missing PKCE.
- OAuth token exchange rejects wrong verifier.
- OAuth code cannot be reused.
- OAuth token exchange rolls back code consumption if agent token activation fails.
- OAuth token endpoint rejects JSON bodies and accepts form-encoded bodies.
- OAuth token exchange rejects wrong `resource`.
- Client metadata document validation rejects metadata whose `client_id` does not exactly match the requested URL.
- Client metadata document validation rejects redirect URIs not listed in metadata.
- MCP POST without an `Accept` header supporting both `application/json` and `text/event-stream` is rejected.
- `WWW-Authenticate` includes protected resource metadata.

Frontend:

- Consent page disables confirm with no project selected.
- Selecting one project enables confirm.
- Selecting `All Projects` enables confirm and makes scope clear.
- Cancel redirects with `access_denied`.
- Agent management page can revoke an authorization.

Integration:

- Start with no token, get discovery challenge, complete browser login/consent, exchange code, call `tools/list`.
- Call `flightrecorder.metrics.summary` for an allowed project.
- Call the same tool for a disallowed project and receive MCP/HTTP forbidden behavior.
- Create/update a project with an all-project authorization.

## Open Decisions

1. Should v1 include dynamic client registration, or only client ID metadata documents plus manual preregistration?
   - Recommendation: support client ID metadata documents first, add dynamic registration if needed for target clients.

2. Should agent authorizations expire by default?
   - Recommendation: yes, 90 days. Users can reauthorize. This limits risk from forgotten local agents.

3. Should scoped agents be able to create projects and then automatically gain access to the newly created project?
   - Recommendation: no. Require `All Projects` for project creation. It is simpler and safer.

4. Should agents be allowed to update feedback report status/labels?
   - Recommendation: not in v1. Query reports only.

5. Should the manual paste-token fallback ship in v1?
   - Recommendation: only if the target MCP client cannot perform OAuth loopback redirects. Prefer standards-compliant code + PKCE.

## Rollout

Because flightrecorder is not production yet:

- Break the telemetry token prefix now from `fr_` to `fr_tel_`.
- Do not support legacy token prefixes.
- Add the tables in a normal forward migration.
- Keep MCP disabled unless `API_BASE_URL` is configured well enough to derive a stable HTTPS MCP resource URL.

Recommended release order:

1. Token prefix and DB migration.
2. OAuth metadata and agent token validation.
3. Consent UI.
4. MCP JSON-RPC tools.
5. Agent management UI.
6. Docs and manual client setup guide.
