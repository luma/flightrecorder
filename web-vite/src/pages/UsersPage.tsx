import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, type AdminInvitationSummary, type AdminUserSummary } from "../api";
import { AdminPageHeader, Input, Panel, Table, copyText } from "../components/AdminPagePrimitives";

export default function UsersPage() {
  const queryClient = useQueryClient();
  const users = useQuery({ queryKey: ["admin-users"], queryFn: api.adminUsers });
  const invitations = useQuery({ queryKey: ["admin-invitations"], queryFn: api.adminInvitations });
  const [inviteEmail, setInviteEmail] = useState("");
  const [createdInvite, setCreatedInvite] = useState("");
  const [copyStatus, setCopyStatus] = useState("");
  const [error, setError] = useState("");

  const toggleUser = useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) => api.setAdminUserEnabled(id, enabled),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["admin-users"] }),
  });
  const createInvite = useMutation({
    mutationFn: () => api.createAdminInvitation(inviteEmail.trim()),
    onMutate: () => {
      setError("");
      setCreatedInvite("");
      setCopyStatus("");
    },
    onSuccess: (resp) => {
      setInviteEmail("");
      setCreatedInvite(resp.token);
      queryClient.invalidateQueries({ queryKey: ["admin-invitations"] });
    },
    onError: (err) => setError(err instanceof Error ? err.message : "Failed to create invitation"),
  });
  const deleteInvite = useMutation({
    mutationFn: (id: string) => api.deleteAdminInvitation(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["admin-invitations"] }),
  });

  return (
    <div className="space-y-5">
      <AdminPageHeader title="Users" />

      <Panel>
        <div className="mb-3 flex flex-wrap items-end gap-2">
          <Input label="Invite user by email" value={inviteEmail} onChange={setInviteEmail} placeholder="admin@example.com" />
          <button
            type="button"
            onClick={() => inviteEmail.trim() && createInvite.mutate()}
            disabled={!inviteEmail.trim() || createInvite.isPending}
            className="btn-primary disabled:opacity-50"
          >
            {createInvite.isPending ? "Inviting..." : "Invite"}
          </button>
          {users.isLoading ? <span className="pb-2 text-sm text-on-surface-variant">Loading...</span> : null}
        </div>
        {error ? <p className="mb-3 text-sm text-status-error">{error}</p> : null}
        {createdInvite ? (
          <div className="mb-3 rounded-md border border-status-warning bg-status-warning-muted p-3 text-sm">
            <p className="font-semibold text-on-surface">Invitation code</p>
            <div className="mt-2 flex gap-2">
              <code className="min-w-0 flex-1 truncate text-on-surface">{createdInvite}</code>
              <button
                type="button"
                onClick={async () => setCopyStatus(await copyText(createdInvite) ? "Copied" : "Copy failed")}
                className="link-button"
              >
                Copy
              </button>
            </div>
            {copyStatus ? <p className="mt-2 text-xs text-on-surface-variant">{copyStatus}</p> : null}
          </div>
        ) : null}
        <Table
          headers={["Email", "Name", "Provider", "Enabled", "Last login", "Created", "Actions"]}
          rows={(users.data?.users ?? []).map((user: AdminUserSummary) => [
            user.email,
            user.name,
            user.provider,
            user.enabled ? "yes" : "no",
            user.last_login_at ?? "",
            user.created_at,
            <button
              type="button"
              onClick={() => toggleUser.mutate({ id: user.id, enabled: !user.enabled })}
              className="link-button"
            >
              {user.enabled ? "Disable" : "Enable"}
            </button>,
          ])}
        />
      </Panel>

      <Panel>
        <div className="mb-3 flex items-center justify-between gap-3">
          <h2 className="text-lg font-semibold text-on-surface">Invitations</h2>
          {invitations.isLoading ? <span className="text-sm text-on-surface-variant">Loading...</span> : null}
        </div>
        <Table
          headers={["Email", "Invited by", "Expires", "Created", "Actions"]}
          rows={(invitations.data?.invitations ?? []).map((invitation: AdminInvitationSummary) => [
            invitation.email,
            invitation.created_by_email ?? "",
            invitation.expires_at,
            invitation.created_at,
            <button
              type="button"
              onClick={() => deleteInvite.mutate(invitation.id)}
              className="link-button"
            >
              Delete
            </button>,
          ])}
        />
      </Panel>
    </div>
  );
}
