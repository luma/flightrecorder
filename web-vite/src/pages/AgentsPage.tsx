import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, type AgentAuthorizationSummary } from "../api";
import {
  AdminPageHeader,
  Badge,
  ConfirmActionButton,
  DateTime,
  Panel,
  Table,
  errorMessage,
  pendingActionID,
} from "../components/AdminPagePrimitives";

export default function AgentsPage() {
  const queryClient = useQueryClient();
  const agentAuthorizations = useQuery({ queryKey: ["agent-authorizations"], queryFn: api.agentAuthorizations });

  const toggleAgentAuthorization = useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) => api.setAgentAuthorizationEnabled(id, enabled),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["agent-authorizations"] }),
  });

  const togglingID = pendingActionID(toggleAgentAuthorization, (variables) => variables.id);

  return (
    <div className="space-y-5">
      <AdminPageHeader title="Agents" />

      <Panel>
        <h2 className="mb-3 text-lg font-semibold text-on-surface">Agent Authorizations</h2>
        {toggleAgentAuthorization.error ? (
          <p className="mb-3 text-sm text-status-error">
            {errorMessage(toggleAgentAuthorization.error, "Failed to update authorization")}
          </p>
        ) : null}
        <Table
          headers={["Client", "Access", "Scopes", "Status", "Created by", "Activated", "Last used", "Actions"]}
          loading={agentAuthorizations.isLoading}
          error={
            agentAuthorizations.error
              ? errorMessage(agentAuthorizations.error, "Failed to load agent authorizations")
              : undefined
          }
          emptyMessage="No agent authorizations yet. They appear here after an MCP client is granted access."
          rows={(agentAuthorizations.data?.authorizations ?? []).map((authorization: AgentAuthorizationSummary) => ({
            key: authorization.id,
            className: authorization.enabled ? undefined : "opacity-60",
            cells: [
              authorization.client_name || authorization.client_id,
              authorization.all_projects ? (
                <Badge tone="info">All projects</Badge>
              ) : (
                authorization.project_keys.join(", ")
              ),
              authorization.scopes.join(", "),
              <Badge tone={authorization.enabled ? "success" : "error"}>
                {authorization.enabled ? "Active" : "Disabled"}
              </Badge>,
              authorization.created_by_email || <span className="text-on-surface-muted">—</span>,
              <DateTime value={authorization.activated_at} fallback="Never" />,
              <DateTime value={authorization.last_used_at} fallback="Never" />,
              <ConfirmActionButton
                label={authorization.enabled ? "Disable" : "Enable"}
                pending={togglingID === authorization.id}
                confirmMessage={
                  authorization.enabled
                    ? `Disable ${authorization.client_name || authorization.client_id}? Requests using this authorization will be rejected immediately.`
                    : undefined
                }
                onConfirm={() =>
                  toggleAgentAuthorization.mutate({ id: authorization.id, enabled: !authorization.enabled })
                }
              />,
            ],
          }))}
        />
      </Panel>
    </div>
  );
}
