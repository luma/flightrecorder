import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, type RejectedEventGroup } from "../api";
import {
  AdminPageHeader,
  Badge,
  DateTime,
  Drawer,
  Panel,
  Table,
  errorMessage,
} from "../components/AdminPagePrimitives";
import { useProjectScope } from "../hooks/useProjectScope";

export const REJECTED_EVENT_COUNT_KEY = "rejected-event-count";

export default function DataQuality() {
  const { projectScope } = useProjectScope();
  const queryClient = useQueryClient();
  const [selected, setSelected] = useState<RejectedEventGroup | null>(null);

  const rejectedEvents = useQuery({
    queryKey: ["rejected-events", projectScope],
    queryFn: () => api.rejectedEvents(projectScope as string),
    enabled: Boolean(projectScope),
  });

  const acknowledge = useMutation({
    mutationFn: () => api.acknowledgeRejectedEvents(projectScope as string),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["rejected-events", projectScope] });
      queryClient.invalidateQueries({ queryKey: [REJECTED_EVENT_COUNT_KEY] });
    },
  });

  const groups = rejectedEvents.data?.groups ?? [];
  const activeCount = rejectedEvents.data?.active_group_count ?? 0;

  if (!projectScope) {
    return (
      <div className="space-y-5">
        <AdminPageHeader title="Data Quality" />
        <Panel>
          <p className="text-sm text-on-surface-variant">
            Select a project to review rejected events.
          </p>
        </Panel>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      <AdminPageHeader title="Data Quality" />

      <Panel>
        <div className="mb-3 flex items-center gap-3">
          <h2 className="text-lg font-semibold text-on-surface">Rejected Events</h2>
          {activeCount > 0 ? <Badge tone="warning">{activeCount} active</Badge> : null}
          <button
            type="button"
            disabled={acknowledge.isPending || activeCount === 0}
            onClick={() => acknowledge.mutate()}
            className="ml-auto btn-secondary"
          >
            {acknowledge.isPending ? "Acknowledging..." : "Acknowledge all"}
          </button>
        </div>

        <p className="mb-3 text-sm text-on-surface-variant">
          Events the collector refused, grouped by type, reason, and game version. These
          usually indicate a game-side bug (wrong field shape or unsupported schema version).
          They are never stored as gameplay events and are pruned after 14 days.
        </p>

        {acknowledge.error ? (
          <p className="mb-3 text-sm text-status-error">
            {errorMessage(acknowledge.error, "Failed to acknowledge")}
          </p>
        ) : null}

        <Table
          headers={[
            "Event type",
            "Reason",
            "Game version",
            "Build channel",
            "Count",
            "First seen",
            "Last seen",
            "",
          ]}
          loading={rejectedEvents.isLoading}
          error={
            rejectedEvents.error
              ? errorMessage(rejectedEvents.error, "Failed to load rejected events")
              : undefined
          }
          emptyMessage="No rejected events. Every event the game sent passed validation."
          rows={groups.map((group) => ({
            key: `${group.event_type}|${group.reason_code}|${group.game_version}|${group.build_channel}`,
            cells: [
              group.event_type || <span className="text-on-surface-muted">—</span>,
              <div>
                <Badge tone="error">{group.reason_code}</Badge>
                <p className="mt-1 text-xs text-on-surface-muted">{group.reason_message}</p>
              </div>,
              group.game_version || <span className="text-on-surface-muted">—</span>,
              group.build_channel || <span className="text-on-surface-muted">—</span>,
              <span className="tabular-nums">{group.event_count}</span>,
              <DateTime value={group.first_seen_at} />,
              <DateTime value={group.last_seen_at} />,
              <button type="button" onClick={() => setSelected(group)} className="link-button">
                View sample
              </button>,
            ],
          }))}
        />
      </Panel>

      {selected ? (
        <Drawer title={`${selected.reason_code} · ${selected.event_type || "unknown"}`} onClose={() => setSelected(null)}>
          <dl className="mb-4 grid grid-cols-2 gap-2 text-sm">
            <dt className="text-on-surface-variant">Reason</dt>
            <dd className="text-on-surface">{selected.reason_message}</dd>
            <dt className="text-on-surface-variant">Game version</dt>
            <dd className="text-on-surface">{selected.game_version || "—"}</dd>
            <dt className="text-on-surface-variant">Build channel</dt>
            <dd className="text-on-surface">{selected.build_channel || "—"}</dd>
            <dt className="text-on-surface-variant">Count</dt>
            <dd className="text-on-surface tabular-nums">{selected.event_count}</dd>
          </dl>
          <h3 className="mb-2 text-sm font-semibold text-on-surface">Sample rejected event</h3>
          <pre className="overflow-auto rounded-md border border-outline-ghost bg-surface-container-lowest p-3 text-xs text-on-surface-variant">
            {JSON.stringify(selected.sample_event, null, 2)}
          </pre>
        </Drawer>
      ) : null}
    </div>
  );
}
