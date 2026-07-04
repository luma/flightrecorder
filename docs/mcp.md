# MCP Remote Agent Access

flightrecorder exposes a remote MCP endpoint for agents that need to inspect or
modify project telemetry configuration.

## Endpoint

Use the deployed API origin plus `/mcp`:

```text
https://telemetry.example.com/mcp
```

The MCP server uses Streamable HTTP JSON-RPC over `POST`. Discovery metadata is
available at:

```text
/.well-known/oauth-protected-resource/mcp
/.well-known/oauth-authorization-server
```

MCP discovery must be reachable at the origin root. Keep `API_BASE_PATH=/` for
deployments that enable MCP.

## Authentication Flow

Agents use OAuth authorization code + PKCE. Reporter ingest tokens do not work
against MCP.

1. The local agent calls `/mcp` without a token.
2. flightrecorder returns a bearer challenge pointing at the protected resource
   metadata.
3. The agent opens the authorization URL in the user's browser, or prints a URL
   that the user can paste into their browser.
4. The browser confirms the user is signed in to the admin UI with Google OAuth
   or the local dev login.
5. flightrecorder shows an access screen with project checkboxes and an
   `All Projects` checkbox.
6. `Confirm` is disabled until at least one project, or `All Projects`, is
   selected.
7. The agent exchanges the authorization code and PKCE verifier for an access
   token.

Agent tokens are prefixed with `fr_agnt_`. Telemetry reporter tokens are
prefixed with `fr_tel_`. Both are stored only as SHA-256 hashes.

## Agent Setup Examples

Use the deployed MCP URL for all examples:

```text
https://telemetry.example.com/mcp
```

For local development, use:

```text
http://localhost:8080/mcp
```

### Claude Code

Claude Code supports remote HTTP MCP servers. Add flightrecorder as an HTTP
server:

```bash
claude mcp add --transport http flightrecorder https://telemetry.example.com/mcp
```

For local development:

```bash
claude mcp add --transport http flightrecorder http://localhost:8080/mcp
```

Then start Claude Code and run:

```text
/mcp
```

Choose `flightrecorder` and follow the browser OAuth flow. flightrecorder will
ask which projects this agent can access. Select one or more projects, or `All
Projects`, then confirm.

Useful Claude Code checks:

```bash
claude mcp list
claude mcp get flightrecorder
```

If you need a fixed OAuth callback port, use Claude Code's OAuth config:

```bash
claude mcp add-json flightrecorder \
  '{"type":"http","url":"https://telemetry.example.com/mcp","oauth":{"callbackPort":8080}}'
```

### Claude.ai Custom Connector

Claude's web UI can connect to remote MCP servers through Custom Connectors.
Use this when you want the connector available in Claude conversations instead
of only Claude Code.

1. Open Claude settings.
2. Go to `Connectors`.
3. Choose `Add custom connector`.
4. Enter the flightrecorder MCP URL:

```text
https://telemetry.example.com/mcp
```

5. Complete the OAuth browser flow and select the allowed flightrecorder
   projects.

Custom Connector availability depends on the Claude plan and workspace policy.

### Codex CLI And IDE

Codex supports Streamable HTTP MCP servers in `config.toml`. Add flightrecorder
with the CLI:

```bash
codex mcp add flightrecorder --url https://telemetry.example.com/mcp
codex mcp login flightrecorder
```

For local development:

```bash
codex mcp add flightrecorder --url http://localhost:8080/mcp
codex mcp login flightrecorder
```

Alternatively, edit `~/.codex/config.toml` or a trusted project-scoped
`.codex/config.toml`:

```toml
[mcp_servers.flightrecorder]
url = "https://telemetry.example.com/mcp"
```

Then run:

```bash
codex mcp login flightrecorder
```

Codex opens a browser for OAuth. After confirming project access in
flightrecorder, use `/mcp` in the Codex TUI to verify the server is connected.

If your environment requires a fixed OAuth callback port, set:

```toml
mcp_oauth_callback_port = 5555

[mcp_servers.flightrecorder]
url = "https://telemetry.example.com/mcp"
```

## Access Model

Agent authorizations are stored separately from admin sessions and telemetry
tokens. They can be disabled from the admin UI under `Users` -> `Agent
Authorizations`.

Agents may:

- Add a project when authorized for `All Projects`.
- Update an existing project they are authorized to access.
- Query project metrics, funnels, event types, reports, and project settings.

Agents may not:

- Add, disable, or delete admin users.
- Create or delete invitations.
- Create, enable, or disable telemetry ingest tokens.
- Create, enable, or disable other agent authorizations.

If the admin user who granted an authorization is disabled, that authorization
is rejected on subsequent requests.

## Tools

The current MCP tool surface is:

- `projects.list`
- `projects.get_settings`
- `projects.create`
- `projects.update`
- `metrics.summary`
- `funnels.query`
- `reports.list`
- `reports.get`
- `events.list`
- `events.types`

Tool inputs use project keys via `project_key`, matching the existing admin API.
`projects.create` and `projects.update` accept the same typed project
configuration shape used by the admin API. `projects.update` first verifies the
project already exists, so a project-scoped agent cannot create an unauthorized
project by updating a new key.

## OAuth Client Registration

MCP clients can use either client ID metadata documents or dynamic client
registration.

For client ID metadata, set `client_id` to an HTTPS URL that returns JSON with
at least:

```json
{
  "client_name": "Local Agent",
  "redirect_uris": ["http://127.0.0.1:49152/callback"]
}
```

flightrecorder fetches that document server-side with redirect limits, response
size limits, and private/link-local IP blocking.

Clients that do not use metadata documents can use dynamic registration:

```http
POST /api/mcp/oauth/register
Content-Type: application/json
```

```json
{
  "client_name": "Local Agent",
  "redirect_uris": ["http://127.0.0.1:49152/callback"]
}
```

The response includes a `client_id` that can be used with:

```text
GET /api/mcp/oauth/authorize
POST /api/mcp/oauth/token
```

The token endpoint accepts `application/x-www-form-urlencoded` OAuth requests
with `grant_type=authorization_code`, `client_id`, `code`, `redirect_uri`,
`code_verifier`, and `resource`.
