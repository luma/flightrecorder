import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, type AdminInvitationSummary, type AdminUserSummary } from "../api";
import {
  AdminPageHeader,
  Badge,
  ConfirmActionButton,
  DateTime,
  Input,
  Panel,
  Table,
  copyText,
  errorMessage,
  pendingActionID,
} from "../components/AdminPagePrimitives";
import { useAuth } from "../auth/AuthContext";

export default function UsersPage() {
  const queryClient = useQueryClient();
  const { user: currentUser } = useAuth();
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
    onError: (err) => setError(errorMessage(err, "Failed to create invitation")),
  });
  const deleteInvite = useMutation({
    mutationFn: (id: string) => api.deleteAdminInvitation(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["admin-invitations"] }),
  });

  const togglingUserID = pendingActionID(toggleUser, (variables) => variables.id);
  const deletingInviteID = pendingActionID(deleteInvite, (id) => id);

  return (
    <div className="space-y-5">
      <AdminPageHeader title="Users" />

      <Panel>
        <form
          className="mb-3 flex flex-wrap items-end gap-2"
          onSubmit={(event) => {
            event.preventDefault();
            if (inviteEmail.trim() && !createInvite.isPending) {
              createInvite.mutate();
            }
          }}
        >
          <Input
            label="Invite user by email"
            value={inviteEmail}
            onChange={setInviteEmail}
            placeholder="admin@example.com"
            type="email"
            autoComplete="off"
          />
          <button
            type="submit"
            disabled={!inviteEmail.trim() || createInvite.isPending}
            className="btn-primary"
          >
            {createInvite.isPending ? "Inviting..." : "Invite"}
          </button>
        </form>
        {error ? <p className="mb-3 text-sm text-status-error">{error}</p> : null}
        {toggleUser.error ? (
          <p className="mb-3 text-sm text-status-error">
            {errorMessage(toggleUser.error, "Failed to update user")}
          </p>
        ) : null}
        {createdInvite ? (
          <div className="mb-3 rounded-md border border-status-warning bg-status-warning-muted p-3 text-sm">
            <p className="font-semibold text-on-surface">Invitation code</p>
            <p className="mt-1 text-xs text-on-surface-variant">Copy it now — it is only shown once.</p>
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
          headers={["Email", "Name", "Provider", "Status", "Last login", "Created", "Actions"]}
          loading={users.isLoading}
          error={users.error ? errorMessage(users.error, "Failed to load users") : undefined}
          emptyMessage="No users yet. Invite one above to get started."
          rows={(users.data?.users ?? []).map((user: AdminUserSummary) => ({
            key: user.id,
            className: user.enabled ? undefined : "opacity-60",
            cells: [
              user.email,
              user.name,
              user.provider,
              <Badge tone={user.enabled ? "success" : "error"}>{user.enabled ? "Active" : "Disabled"}</Badge>,
              <DateTime value={user.last_login_at} fallback="Never" />,
              <DateTime value={user.created_at} />,
              currentUser?.email === user.email ? (
                <span className="text-xs text-on-surface-muted" title="You cannot disable your own account">
                  You
                </span>
              ) : (
                <ConfirmActionButton
                  label={user.enabled ? "Disable" : "Enable"}
                  pending={togglingUserID === user.id}
                  confirmMessage={
                    user.enabled ? `Disable ${user.email}? They will lose access immediately.` : undefined
                  }
                  onConfirm={() => toggleUser.mutate({ id: user.id, enabled: !user.enabled })}
                />
              ),
            ],
          }))}
        />
      </Panel>

      <Panel>
        <h2 className="mb-3 text-lg font-semibold text-on-surface">Invitations</h2>
        {deleteInvite.error ? (
          <p className="mb-3 text-sm text-status-error">
            {errorMessage(deleteInvite.error, "Failed to delete invitation")}
          </p>
        ) : null}
        <Table
          headers={["Email", "Invited by", "Expires", "Created", "Actions"]}
          loading={invitations.isLoading}
          error={invitations.error ? errorMessage(invitations.error, "Failed to load invitations") : undefined}
          emptyMessage="No pending invitations. Codes appear here until they are accepted or expire."
          rows={(invitations.data?.invitations ?? []).map((invitation: AdminInvitationSummary) => {
            const expired = Date.parse(invitation.expires_at) < Date.now();
            return {
              key: invitation.id,
              className: expired ? "opacity-60" : undefined,
              cells: [
                invitation.email,
                invitation.created_by_email || <span className="text-on-surface-muted">—</span>,
                <span className="flex items-center gap-2">
                  <DateTime value={invitation.expires_at} />
                  {expired ? <Badge tone="warning">Expired</Badge> : null}
                </span>,
                <DateTime value={invitation.created_at} />,
                <ConfirmActionButton
                  label="Delete"
                  pending={deletingInviteID === invitation.id}
                  pendingLabel="Deleting..."
                  confirmMessage={`Delete the invitation for ${invitation.email}? Its code will stop working.`}
                  onConfirm={() => deleteInvite.mutate(invitation.id)}
                />,
              ],
            };
          })}
        />
      </Panel>
    </div>
  );
}
