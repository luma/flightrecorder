import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, type AgentAuthorizationSummary } from "../api";
import { AdminPageHeader, Panel, Table } from "../components/AdminPagePrimitives";

export default function AgentsPage() {
  const queryClient = useQueryClient();
  const agentAuthorizations = useQuery({ queryKey: ["agent-authorizations"], queryFn: api.agentAuthorizations });

  const toggleAgentAuthorization = useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) => api.setAgentAuthorizationEnabled(id, enabled),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["agent-authorizations"] }),
  });

  return (
    <div className="space-y-5">
      <AdminPageHeader title="Agents" />

      <Panel>
        <div className="mb-3 flex items-center justify-between gap-3">
          <h2 className="text-lg font-semibold text-on-surface">Agent Authorizations</h2>
          {agentAuthorizations.isLoading ? <span className="text-sm text-on-surface-variant">Loading...</span> : null}
        </div>
        <Table
          headers={["Client", "Access", "Scopes", "Enabled", "Created by", "Activated", "Last used", "Actions"]}
          rows={(agentAuthorizations.data?.authorizations ?? []).map((authorization: AgentAuthorizationSummary) => [
            authorization.client_name || authorization.client_id,
            authorization.all_projects ? "All Projects" : authorization.project_keys.join(", "),
            authorization.scopes.join(", "),
            authorization.enabled ? "yes" : "no",
            authorization.created_by_email ?? "",
            authorization.activated_at ?? "",
            authorization.last_used_at ?? "",
            <button
              type="button"
              onClick={() => toggleAgentAuthorization.mutate({ id: authorization.id, enabled: !authorization.enabled })}
              className="link-button"
            >
              {authorization.enabled ? "Disable" : "Enable"}
            </button>,
          ])}
        />
      </Panel>
    </div>
  );
}
