DROP INDEX IF EXISTS mcp_oauth_codes_expires_at_idx;
DROP INDEX IF EXISTS mcp_oauth_codes_agent_authorization_idx;
DROP TABLE IF EXISTS mcp_oauth_codes;

DROP TABLE IF EXISTS mcp_oauth_clients;

DROP INDEX IF EXISTS agent_authorization_projects_project_idx;
DROP TABLE IF EXISTS agent_authorization_projects;

DROP INDEX IF EXISTS agent_authorizations_created_at_idx;
DROP INDEX IF EXISTS agent_authorizations_created_by_idx;
DROP INDEX IF EXISTS agent_authorizations_token_hash_idx;
DROP TABLE IF EXISTS agent_authorizations;
