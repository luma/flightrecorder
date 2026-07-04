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

CREATE INDEX agent_authorizations_created_by_idx
ON agent_authorizations(created_by_admin_user_id);

CREATE INDEX agent_authorizations_created_at_idx
ON agent_authorizations(created_at DESC);

CREATE TABLE agent_authorization_projects (
    agent_authorization_id uuid NOT NULL REFERENCES agent_authorizations(id) ON DELETE CASCADE,
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    PRIMARY KEY (agent_authorization_id, project_id)
);

CREATE INDEX agent_authorization_projects_project_idx
ON agent_authorization_projects(project_id);

CREATE TABLE mcp_oauth_clients (
    client_id text PRIMARY KEY,
    client_name text NOT NULL,
    redirect_uris text[] NOT NULL,
    client_uri text NOT NULL DEFAULT '',
    logo_uri text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
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

CREATE INDEX mcp_oauth_codes_agent_authorization_idx
ON mcp_oauth_codes(agent_authorization_id);

CREATE INDEX mcp_oauth_codes_expires_at_idx
ON mcp_oauth_codes(expires_at);
