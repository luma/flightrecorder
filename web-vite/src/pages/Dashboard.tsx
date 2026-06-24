import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  api,
  type AdminFilters,
  type EventSummary,
  type EventTypeSummary,
  type FunnelSummary,
  type HeatmapCell,
  type QueryField,
  type ReportDetail,
  type ReportSummary,
  type TraceEvent,
} from "../api";
import { useProjectScope } from "../hooks/useProjectScope";

const tabs = [
  "Overview",
  "Events",
  "Trace",
  "Galaxy",
  "Zone",
  "Funnels",
  "Reports",
  "Schema",
  "Settings",
] as const;

const reportStatuses = ["new", "seen", "needs_more_info", "reproduced", "fixed", "wont_fix"];
const timePresets = [
  ["24h", 1],
  ["7d", 7],
  ["30d", 30],
  ["all", 0],
] as const;

type Tab = (typeof tabs)[number];
type ExportFormat = "json" | "csv" | "ndjson";

interface Filters {
  projectID: string;
  from: string;
  to: string;
  eventType: string;
  systemID: string;
  zoneID: string;
  commanderID: string;
  gameVersion: string;
  buildChannel: string;
  fieldKey: string;
  fieldValue: string;
  reportStatus: string;
  reportLabel: string;
}

export default function Dashboard() {
  const { projectScope, setProjectScope } = useProjectScope();
  const initial = useMemo(() => filtersFromURL(projectScope || "sursidus"), []);
  const [activeTab, setActiveTab] = useState<Tab>(tabFromURL());
  const [filters, setFilters] = useState<Filters>(initial);
  const [selectedEvent, setSelectedEvent] = useState<EventSummary | null>(null);
  const [selectedReportID, setSelectedReportID] = useState<string | null>(null);

  useEffect(() => {
    setProjectScope(filters.projectID || null);
    const params = new URLSearchParams();
    params.set("tab", activeTab);
    params.set("project_id", filters.projectID);
    setParam(params, "from", filters.from);
    setParam(params, "to", filters.to);
    setParam(params, "event_type", filters.eventType);
    setParam(params, "system_id", filters.systemID);
    setParam(params, "zone_id", filters.zoneID);
    setParam(params, "commander_id", filters.commanderID);
    setParam(params, "game_version", filters.gameVersion);
    setParam(params, "build_channel", filters.buildChannel);
    setParam(params, "field_key", filters.fieldKey);
    setParam(params, "field_value", filters.fieldValue);
    setParam(params, "status", filters.reportStatus);
    setParam(params, "label", filters.reportLabel);
    window.history.replaceState(null, "", `${window.location.pathname}?${params.toString()}`);
  }, [activeTab, filters, setProjectScope]);

  const adminFilters = useMemo(() => toAdminFilters(filters), [filters]);

  const summary = useQuery({
    queryKey: ["summary", adminFilters],
    queryFn: () => api.summary(adminFilters),
  });
  const events = useQuery({
    queryKey: ["events", adminFilters],
    queryFn: () => api.events(adminFilters),
  });
  const reports = useQuery({
    queryKey: ["reports", adminFilters],
    queryFn: () => api.reports(adminFilters),
  });
  const eventTypes = useQuery({
    queryKey: ["event-types", filters.projectID],
    queryFn: () => api.eventTypes(filters.projectID),
  });
  const settings = useQuery({
    queryKey: ["settings", filters.projectID],
    queryFn: () => api.settings(filters.projectID),
  });
  const systemHeatmap = useQuery({
    queryKey: ["system-heatmap", adminFilters],
    queryFn: () => api.systemHeatmap(adminFilters),
  });
  const firstSystem = filters.systemID || systemHeatmap.data?.cells[0]?.system_id || "lave";
  const zoneHeatmap = useQuery({
    queryKey: ["zone-heatmap", adminFilters, firstSystem],
    queryFn: () => api.zoneHeatmap({ ...adminFilters, system_id: firstSystem }),
  });
  const funnels = useQuery({
    queryKey: ["funnels", adminFilters],
    queryFn: () => api.funnels(adminFilters),
  });

  const selectedCommander = filters.commanderID || events.data?.events[0]?.commander_id || "";
  const trace = useQuery({
    queryKey: ["trace", filters.projectID, selectedCommander],
    queryFn: () =>
      selectedCommander
        ? api.commanderTrace(filters.projectID, selectedCommander)
        : Promise.resolve({ events: [] }),
  });

  const selectedReport = useQuery({
    enabled: !!selectedReportID,
    queryKey: ["report-detail", filters.projectID, selectedReportID],
    queryFn: () => api.reportDetail(filters.projectID, selectedReportID || ""),
  });

  const currentRows = useMemo(() => {
    if (activeTab === "Reports") return reports.data?.reports ?? [];
    if (activeTab === "Schema") return eventTypes.data?.event_types ?? [];
    if (activeTab === "Funnels") return funnels.data?.funnels ?? [];
    if (activeTab === "Galaxy") return systemHeatmap.data?.cells ?? [];
    if (activeTab === "Zone") return zoneHeatmap.data?.cells ?? [];
    if (activeTab === "Trace") return trace.data?.events ?? [];
    return events.data?.events ?? [];
  }, [activeTab, eventTypes.data, events.data, funnels.data, reports.data, systemHeatmap.data, trace.data, zoneHeatmap.data]);

  const exportRows = (format: ExportFormat) => {
    exportData(`${activeTab.toLowerCase()}-${filters.projectID}.${format}`, currentRows, format, adminFilters);
  };

  const setFilter = <K extends keyof Filters>(key: K, value: Filters[K]) => {
    setFilters((prev) => ({ ...prev, [key]: value }));
  };

  return (
    <div className="space-y-5">
      <header className="flex flex-wrap items-center gap-3">
        <div>
          <p className="text-sm uppercase tracking-wide text-on-surface-variant">Collector Console</p>
          <h1 className="text-3xl font-bold text-on-surface">flightrecorder</h1>
        </div>
        <div className="ml-auto flex flex-wrap gap-2">
          <button type="button" onClick={copyPermalink} className="btn-secondary">Copy Link</button>
          <button type="button" onClick={() => exportRows("json")} className="btn-secondary">JSON</button>
          <button type="button" onClick={() => exportRows("csv")} className="btn-secondary">CSV</button>
          <button type="button" onClick={() => exportRows("ndjson")} className="btn-secondary">NDJSON</button>
        </div>
      </header>

      <FilterBar
        filters={filters}
        setFilter={setFilter}
        eventTypes={eventTypes.data?.event_types ?? []}
        queryFields={settings.data?.project.query_fields ?? []}
      />

      <nav className="flex flex-wrap gap-2">
        {tabs.map((tab) => (
          <button
            key={tab}
            type="button"
            onClick={() => setActiveTab(tab)}
            className={activeTab === tab ? "btn-primary" : "btn-secondary"}
          >
            {tab}
          </button>
        ))}
      </nav>

      {activeTab === "Overview" ? <Overview summary={summary.data} loading={summary.isLoading} /> : null}
      {activeTab === "Events" ? (
        <EventsTable
          events={events.data?.events ?? []}
          queryFields={settings.data?.project.query_fields ?? []}
          onOpen={setSelectedEvent}
          onTrace={(commanderID) => {
            setFilter("commanderID", commanderID);
            setActiveTab("Trace");
          }}
        />
      ) : null}
      {activeTab === "Trace" ? (
        <TraceTable
          commanderID={selectedCommander}
          events={trace.data?.events ?? []}
          queryFields={settings.data?.project.query_fields ?? []}
        />
      ) : null}
      {activeTab === "Galaxy" ? (
        <HeatmapTable
          cells={systemHeatmap.data?.cells ?? []}
          onSelect={(cell) => {
            setFilter("systemID", cell.system_id);
            setFilter("eventType", cell.event_type);
            setActiveTab("Zone");
          }}
        />
      ) : null}
      {activeTab === "Zone" ? (
        <HeatmapTable
          cells={zoneHeatmap.data?.cells ?? []}
          onSelect={(cell) => {
            setFilter("zoneID", cell.zone_id ?? "");
            setFilter("eventType", cell.event_type);
          }}
        />
      ) : null}
      {activeTab === "Funnels" ? <FunnelsTable funnels={funnels.data?.funnels ?? []} /> : null}
      {activeTab === "Reports" ? (
        <ReportsTable
          reports={reports.data?.reports ?? []}
          onOpen={(report) => setSelectedReportID(report.report_id)}
          onTrace={(commanderID) => {
            setFilter("commanderID", commanderID);
            setActiveTab("Trace");
          }}
        />
      ) : null}
      {activeTab === "Schema" ? (
        <SchemaTable
          eventTypes={eventTypes.data?.event_types ?? []}
          queryFields={settings.data?.project.query_fields ?? []}
        />
      ) : null}
      {activeTab === "Settings" ? <Settings projectID={filters.projectID} settings={settings.data} /> : null}

      <EventDrawer
        event={selectedEvent}
        queryFields={settings.data?.project.query_fields ?? []}
        onClose={() => setSelectedEvent(null)}
      />
      <ReportDrawer
        projectID={filters.projectID}
        report={selectedReport.data}
        loading={selectedReport.isLoading}
        onClose={() => setSelectedReportID(null)}
        onTrace={(commanderID) => {
          setFilter("commanderID", commanderID);
          setActiveTab("Trace");
          setSelectedReportID(null);
        }}
      />
    </div>
  );
}

function FilterBar({
  filters,
  setFilter,
  eventTypes,
  queryFields,
}: {
  filters: Filters;
  setFilter: <K extends keyof Filters>(key: K, value: Filters[K]) => void;
  eventTypes: EventTypeSummary[];
  queryFields: QueryField[];
}) {
  return (
    <Panel>
      <div className="grid gap-3 md:grid-cols-4">
        <Input label="Project" value={filters.projectID} onChange={(value) => setFilter("projectID", value || "sursidus")} />
        <Select
          label="Range"
          value={rangeLabel(filters)}
          options={timePresets.map(([label]) => label)}
          onChange={(value) => applyPreset(value, setFilter)}
        />
        <Input label="From" value={filters.from} onChange={(value) => setFilter("from", value)} placeholder="RFC3339" />
        <Input label="To" value={filters.to} onChange={(value) => setFilter("to", value)} placeholder="RFC3339" />
        <Select
          label="Event"
          value={filters.eventType}
          options={["", ...eventTypes.map((eventType) => eventType.event_type)]}
          onChange={(value) => setFilter("eventType", value)}
        />
        <Input label="System" value={filters.systemID} onChange={(value) => setFilter("systemID", value)} />
        <Input label="Zone" value={filters.zoneID} onChange={(value) => setFilter("zoneID", value)} />
        <Input label="Commander" value={filters.commanderID} onChange={(value) => setFilter("commanderID", value)} />
        <Select
          label="Field"
          value={filters.fieldKey}
          options={["", ...queryFields.filter((field) => field.filterable).map((field) => field.key)]}
          onChange={(value) => setFilter("fieldKey", value)}
        />
        <Input label="Field value" value={filters.fieldValue} onChange={(value) => setFilter("fieldValue", value)} />
        <Input label="Version" value={filters.gameVersion} onChange={(value) => setFilter("gameVersion", value)} />
        <Input label="Channel" value={filters.buildChannel} onChange={(value) => setFilter("buildChannel", value)} />
        <Select
          label="Report status"
          value={filters.reportStatus}
          options={["", ...reportStatuses]}
          onChange={(value) => setFilter("reportStatus", value)}
        />
        <Input label="Report label" value={filters.reportLabel} onChange={(value) => setFilter("reportLabel", value)} />
      </div>
    </Panel>
  );
}

function Overview({
  summary,
  loading,
}: {
  summary?: Awaited<ReturnType<typeof api.summary>>;
  loading: boolean;
}) {
  if (loading) return <Panel>Loading...</Panel>;
  const metrics = [
    ["Events", summary?.event_count ?? 0],
    ["Commanders", summary?.commander_count ?? 0],
    ["Sessions", summary?.session_count ?? 0],
    ["Deaths", summary?.death_count ?? 0],
    ["Reports", summary?.report_count ?? 0],
    ["Opt-ins", summary?.opt_in_count ?? 0],
  ];
  return (
    <div className="grid gap-3 md:grid-cols-3">
      {metrics.map(([label, value]) => (
        <Panel key={label}>
          <p className="text-sm text-on-surface-variant">{label}</p>
          <p className="mt-2 text-3xl font-bold text-on-surface">{value}</p>
        </Panel>
      ))}
    </div>
  );
}

function EventsTable({
  events,
  queryFields,
  onOpen,
  onTrace,
}: {
  events: EventSummary[];
  queryFields: QueryField[];
  onOpen: (event: EventSummary) => void;
  onTrace: (commanderID: string) => void;
}) {
  const visibleFields = queryFields.slice(0, 4);
  return (
    <Table
      headers={["Type", "Commander", "System", "Zone", ...visibleFields.map((field) => field.label || field.key), "Version", "Time", "Actions"]}
      rows={events.map((event) => [
        event.event_type,
        event.commander_id,
        event.system_id,
        event.zone_id,
        ...visibleFields.map((field) => formatFieldValue(event.fields?.[field.key])),
        event.game_version,
        event.real_ts,
        <div className="flex gap-2">
          <button type="button" onClick={() => onOpen(event)} className="link-button">Open</button>
          <button type="button" onClick={() => onTrace(event.commander_id)} className="link-button">Trace</button>
          <button type="button" onClick={() => copyText(event.id)} className="link-button">Copy ID</button>
        </div>,
      ])}
    />
  );
}

function TraceTable({ commanderID, events, queryFields }: { commanderID?: string; events: TraceEvent[]; queryFields?: QueryField[] }) {
  const visibleFields = (queryFields ?? []).slice(0, 3);
  return (
    <section className="space-y-3">
      <div className="flex items-center gap-2 text-sm text-on-surface-variant">
        <span>{commanderID || "No commander selected"}</span>
        {commanderID ? <button type="button" onClick={() => copyText(commanderID)} className="link-button">Copy Commander</button> : null}
      </div>
      <Table
        headers={["Type", "System", "Zone", ...visibleFields.map((field) => field.label || field.key), "Game time", "Time", "Payload"]}
        rows={events.map((event) => [
          event.event_type,
          event.system_id,
          event.zone_id,
          ...visibleFields.map((field) => formatFieldValue(event.fields?.[field.key])),
          String(event.game_time),
          event.real_ts,
          JSON.stringify(event.payload),
        ])}
      />
    </section>
  );
}

function HeatmapTable({ cells, onSelect }: { cells: HeatmapCell[]; onSelect: (cell: HeatmapCell) => void }) {
  return (
    <Table
      headers={["System", "Zone", "Grid", "Type", "Count", "Actions"]}
      rows={cells.map((cell) => [
        cell.system_id,
        cell.zone_id ?? "",
        cell.grid_x === undefined ? "" : `${cell.grid_x}, ${cell.grid_z}`,
        cell.event_type,
        String(cell.event_count),
        <button type="button" onClick={() => onSelect(cell)} className="link-button">Filter</button>,
      ])}
    />
  );
}

function FunnelsTable({ funnels }: { funnels: FunnelSummary[] }) {
  return (
    <Table
      headers={["Name", "Description", "Started", "Completed", "Rate", "Drop-off"]}
      rows={funnels.map((funnel) => [
        funnel.name,
        funnel.description,
        String(funnel.started),
        String(funnel.completed),
        `${Math.round(funnel.rate * 100)}%`,
        funnel.dropoff,
      ])}
    />
  );
}

function ReportsTable({
  reports,
  onOpen,
  onTrace,
}: {
  reports: ReportSummary[];
  onOpen: (report: ReportSummary) => void;
  onTrace: (commanderID: string) => void;
}) {
  return (
    <Table
      headers={["Details", "Report", "Status", "Labels", "Mood", "Notes", "Commander", "System", "Created", "Trace"]}
      rows={reports.map((report) => [
        <button type="button" onClick={() => onOpen(report)} className="btn-secondary">Open</button>,
        <button type="button" onClick={() => onOpen(report)} className="link-button">{report.report_id}</button>,
        report.status,
        report.labels.join(", "),
        report.mood_label,
        report.notes_preview,
        report.commander_id,
        report.system_id,
        report.created_at,
        <button type="button" onClick={() => onTrace(report.commander_id)} className="link-button">Trace</button>,
      ])}
    />
  );
}

function SchemaTable({ eventTypes, queryFields }: { eventTypes: EventTypeSummary[]; queryFields: QueryField[] }) {
  return (
    <div className="space-y-4">
      <Table
        headers={["Event type", "Count", "Last seen", "Sample payload"]}
        rows={eventTypes.map((eventType) => [
          eventType.event_type,
          String(eventType.event_count),
          eventType.last_seen_at,
          JSON.stringify(eventType.sample_payload),
        ])}
      />
      <Table
        headers={["Field", "Label", "Source", "Type", "Filter", "Aggregations"]}
        rows={queryFields.map((field) => [
          field.key,
          field.label,
          field.source,
          field.type,
          field.filterable ? "yes" : "no",
          field.aggregations.join(", "),
        ])}
      />
    </div>
  );
}

function Settings({
  projectID,
  settings,
}: {
  projectID: string;
  settings?: Awaited<ReturnType<typeof api.settings>>;
}) {
  const queryClient = useQueryClient();
  const [tokenName, setTokenName] = useState("");
  const [createdToken, setCreatedToken] = useState("");
  const [tokenCopyStatus, setTokenCopyStatus] = useState("");
  const trimmedTokenName = tokenName.trim();
  const createToken = useMutation({
    mutationFn: () => api.createIngestToken(projectID, trimmedTokenName),
    onMutate: () => {
      setCreatedToken("");
      setTokenCopyStatus("");
    },
    onSuccess: (resp) => {
      setCreatedToken(resp.token);
      setTokenName("");
      queryClient.invalidateQueries({ queryKey: ["settings", projectID] });
    },
  });
  const toggleToken = useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) =>
      api.setIngestTokenEnabled(projectID, id, enabled),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["settings", projectID] }),
  });

  return (
    <div className="space-y-4">
      <Panel>
        <dl className="grid gap-3 text-sm md:grid-cols-2">
          <Info label="Project" value={settings?.project.project_id ?? projectID} />
          <Info label="Display name" value={settings?.project.display_name ?? ""} />
          <Info label="Validation" value={settings?.project.validation_mode ?? ""} />
          <Info label="Session" value="Signed admin cookie" />
          <Info label="Admin allowlist" value="ADMIN_ALLOWED_EMAILS" />
          <Info label="Local screenshots" value="REPORT_STORAGE_BACKEND=local, REPORT_STORAGE_DIR=var/reports" />
          <Info label="R2 screenshots" value="REPORT_STORAGE_BACKEND=r2 with R2_ENDPOINT, R2_BUCKET, R2_ACCESS_KEY_ID" />
        </dl>
      </Panel>
      <Panel>
        <form
          className="mb-3 flex flex-wrap items-end gap-2"
          onSubmit={(event) => {
            event.preventDefault();
            if (trimmedTokenName && !createToken.isPending) {
              createToken.mutate();
            }
          }}
        >
          <Input label="Token name" value={tokenName} onChange={setTokenName} placeholder="sursidus-dev" />
          <button type="submit" disabled={!trimmedTokenName || createToken.isPending} className="btn-primary disabled:opacity-50">
            {createToken.isPending ? "Creating..." : "Create Token"}
          </button>
        </form>
        {createToken.error ? (
          <p className="mb-3 text-sm text-status-error">
            {createToken.error instanceof Error ? createToken.error.message : "Failed to create token"}
          </p>
        ) : null}
        {createdToken ? (
          <div className="mb-3 rounded-md border border-status-warning bg-status-warning-muted p-3 text-sm">
            <p className="font-semibold text-on-surface">New token</p>
            <div className="mt-2 flex gap-2">
              <code className="min-w-0 flex-1 truncate text-on-surface">{createdToken}</code>
              <button
                type="button"
                onClick={async () => {
                  setTokenCopyStatus(await copyText(createdToken) ? "Copied" : "Copy failed");
                }}
                className="link-button"
              >
                Copy
              </button>
            </div>
            {tokenCopyStatus ? <p className="mt-2 text-xs text-on-surface-variant">{tokenCopyStatus}</p> : null}
          </div>
        ) : null}
        <Table
          headers={["Name", "Enabled", "Last used", "Created", "Actions"]}
          rows={(settings?.tokens ?? []).map((token) => [
            token.name,
            token.enabled ? "yes" : "no",
            token.last_used_at ?? "",
            token.created_at,
            <button
              type="button"
              onClick={() => toggleToken.mutate({ id: token.id, enabled: !token.enabled })}
              className="link-button"
            >
              {token.enabled ? "Disable" : "Enable"}
            </button>,
          ])}
        />
      </Panel>
      <Panel>
        <pre className="max-h-80 overflow-auto text-xs text-on-surface">
          {JSON.stringify(settings?.project ?? {}, null, 2)}
        </pre>
      </Panel>
    </div>
  );
}

function EventDrawer({
  event,
  queryFields,
  onClose,
}: {
  event: EventSummary | null;
  queryFields: QueryField[];
  onClose: () => void;
}) {
  if (!event) return null;
  const fieldRows: Array<[string, string]> = queryFields
    .filter((field) => event.fields && Object.prototype.hasOwnProperty.call(event.fields, field.key))
    .map((field) => [field.label || field.key, formatFieldValue(event.fields[field.key])]);
  return (
    <Drawer title={event.event_type} onClose={onClose}>
      <div className="space-y-3 text-sm">
        <InfoGrid
          rows={[
            ["Event ID", event.id],
            ["Commander", event.commander_id],
            ["System", event.system_id],
            ["Zone", event.zone_id],
            ["Version", event.game_version],
            ["Channel", event.build_channel],
            ["Commit", event.commit_sha || "-"],
          ]}
        />
        {fieldRows.length > 0 ? <InfoGrid rows={fieldRows} /> : null}
        <div className="flex flex-wrap gap-2">
          <button type="button" onClick={() => copyText(event.id)} className="btn-secondary">Copy Event ID</button>
          <button type="button" onClick={() => copyText(event.commander_id)} className="btn-secondary">Copy Commander</button>
          <button type="button" onClick={() => copyText(JSON.stringify(event, null, 2))} className="btn-secondary">Copy JSON</button>
        </div>
        <JSONBlock label="Context" value={event.context} />
        <JSONBlock label="Metrics" value={event.metrics} />
        <JSONBlock label="Dimensions" value={event.dimensions} />
        <JSONBlock label="Payload" value={event.payload} />
        <pre className="max-h-96 overflow-auto rounded-md bg-surface-container p-3 text-xs text-on-surface">
          {JSON.stringify(event.event_json, null, 2)}
        </pre>
      </div>
    </Drawer>
  );
}

function ReportDrawer({
  projectID,
  report,
  loading,
  onClose,
  onTrace,
}: {
  projectID: string;
  report?: ReportDetail;
  loading: boolean;
  onClose: () => void;
  onTrace: (commanderID: string) => void;
}) {
  const queryClient = useQueryClient();
  const [status, setStatus] = useState("");
  const [labels, setLabels] = useState("");
  const [note, setNote] = useState("");

  useEffect(() => {
    setStatus(report?.status ?? "");
    setLabels(report?.labels.join(", ") ?? "");
    setNote("");
  }, [report]);

  const mutation = useMutation({
    mutationFn: () =>
      report
        ? api.updateReport(projectID, report.report_id, {
            status,
            labels: splitLabels(labels),
            note,
          })
        : Promise.reject(new Error("missing report")),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["reports"] });
      queryClient.invalidateQueries({ queryKey: ["report-detail", projectID, report?.report_id] });
      setNote("");
    },
  });

  if (!report && !loading) return null;
  return (
    <Drawer title={report ? `Report ${report.report_id}` : "Report"} onClose={onClose}>
      {loading || !report ? (
        <p className="text-sm text-on-surface-variant">Loading...</p>
      ) : (
        <div className="space-y-4 text-sm">
          <InfoGrid
            rows={[
              ["Status", report.status],
              ["Mood", `${report.mood} ${report.mood_label}`],
              ["Commander", report.commander_id],
              ["System", report.system_id],
              ["Zone", report.zone_id],
              ["Created", report.created_at],
            ]}
          />
          <section className="space-y-2">
            <h3 className="font-semibold text-on-surface">Screenshot</h3>
            {report.screenshot_object_key ? (
              <a href={api.reportScreenshotURL(projectID, report.report_id)} target="_blank" rel="noreferrer" className="block rounded-md border border-outline-ghost bg-surface-container p-2">
                <img src={api.reportScreenshotURL(projectID, report.report_id)} alt="Submitted report screenshot" className="max-h-[32rem] w-full rounded-md object-contain" />
              </a>
            ) : (
              <p className="rounded-md border border-outline-ghost bg-surface-container p-3 text-on-surface-variant">No screenshot was submitted with this report.</p>
            )}
          </section>
          <div className="grid gap-3 md:grid-cols-2">
            <Select label="Status" value={status} options={reportStatuses} onChange={setStatus} />
            <Input label="Labels" value={labels} onChange={setLabels} placeholder="ui, onboarding" />
          </div>
          <label className="block text-sm text-on-surface-variant">
            Note
            <textarea value={note} onChange={(event) => setNote(event.target.value)} className="mt-1 h-24 w-full rounded-md border border-outline-ghost bg-surface-container px-2 py-1 text-on-surface outline-none focus:border-primary" />
          </label>
          <div className="flex flex-wrap gap-2">
            <button type="button" onClick={() => mutation.mutate()} className="btn-primary">Save</button>
            <button type="button" onClick={() => onTrace(report.commander_id)} className="btn-secondary">Trace Commander</button>
            <button type="button" onClick={() => copyText(report.commander_id)} className="btn-secondary">Copy Commander</button>
          </div>
          <JSONBlock label="Context" value={report.context} />
          <JSONBlock label="Metrics" value={report.metrics} />
          <JSONBlock label="Dimensions" value={report.dimensions} />
          <JSONBlock label="Payload" value={report.payload} />
          <section className="space-y-2">
            <h3 className="font-semibold text-on-surface">Notes</h3>
            {report.notes.length === 0 ? <p className="text-on-surface-variant">No notes</p> : null}
            {report.notes.map((item) => (
              <div key={item.id} className="rounded-md border border-outline-ghost p-2">
                <p className="text-on-surface">{item.note}</p>
                <p className="text-xs text-on-surface-variant">{item.created_at}</p>
              </div>
            ))}
          </section>
          <TraceTable commanderID={report.commander_id} events={report.trace} />
        </div>
      )}
    </Drawer>
  );
}

function Panel({ children }: { children: ReactNode }) {
  return <section className="rounded-md border border-outline-ghost bg-surface-container-low p-4">{children}</section>;
}

function Drawer({ title, children, onClose }: { title: string; children: ReactNode; onClose: () => void }) {
  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-black/40">
      <aside className="h-full w-full max-w-5xl overflow-auto border-l border-outline-ghost bg-surface-container-lowest p-5 shadow-xl">
        <div className="mb-4 flex items-center gap-3">
          <h2 className="text-xl font-bold text-on-surface">{title}</h2>
          <button type="button" onClick={onClose} className="ml-auto btn-secondary">Close</button>
        </div>
        {children}
      </aside>
    </div>
  );
}

function Table({ headers, rows }: { headers: string[]; rows: ReactNode[][] }) {
  return (
    <div className="overflow-x-auto rounded-md border border-outline-ghost">
      <table className="min-w-full divide-y divide-outline-ghost text-left text-sm">
        <thead className="bg-surface-container">
          <tr>
            {headers.map((header) => (
              <th key={header} className="px-3 py-2 font-semibold text-on-surface">{header}</th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y divide-outline-ghost bg-surface-container-lowest">
          {rows.length === 0 ? (
            <tr>
              <td className="px-3 py-4 text-on-surface-variant" colSpan={headers.length}>No rows</td>
            </tr>
          ) : (
            rows.map((row, index) => (
              <tr key={index}>
                {row.map((cell, cellIndex) => (
                  <td key={cellIndex} className="max-w-xs truncate px-3 py-2 text-on-surface-variant">{cell}</td>
                ))}
              </tr>
            ))
          )}
        </tbody>
      </table>
    </div>
  );
}

function Input({ label, value, onChange, placeholder }: { label: string; value: string; onChange: (value: string) => void; placeholder?: string }) {
  return (
    <label className="block text-sm text-on-surface-variant">
      {label}
      <input value={value} placeholder={placeholder} onChange={(event) => onChange(event.target.value)} className="mt-1 w-full rounded-md border border-outline-ghost bg-surface-container px-2 py-1 text-on-surface outline-none focus:border-primary" />
    </label>
  );
}

function Select({ label, value, options, onChange }: { label: string; value: string; options: readonly string[]; onChange: (value: string) => void }) {
  return (
    <label className="block text-sm text-on-surface-variant">
      {label}
      <select value={value} onChange={(event) => onChange(event.target.value)} className="mt-1 w-full rounded-md border border-outline-ghost bg-surface-container px-2 py-1 text-on-surface outline-none focus:border-primary">
        {options.map((option) => (
          <option key={option || "any"} value={option}>{option || "any"}</option>
        ))}
      </select>
    </label>
  );
}

function Info({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-on-surface-variant">{label}</dt>
      <dd className="text-on-surface">{value}</dd>
    </div>
  );
}

function InfoGrid({ rows }: { rows: Array<[string, string]> }) {
  return (
    <dl className="grid gap-2 md:grid-cols-2">
      {rows.map(([label, value]) => (
        <Info key={label} label={label} value={value} />
      ))}
    </dl>
  );
}

function JSONBlock({ label, value }: { label: string; value?: Record<string, unknown> }) {
  return (
    <div>
      <p className="mb-1 text-xs uppercase tracking-wide text-on-surface-variant">{label}</p>
      <pre className="max-h-48 overflow-auto rounded-md bg-surface-container p-3 text-xs text-on-surface">
        {JSON.stringify(value ?? {}, null, 2)}
      </pre>
    </div>
  );
}

function formatFieldValue(value: unknown): string {
  if (value === null || value === undefined) return "";
  if (typeof value === "number") return Number.isInteger(value) ? String(value) : value.toFixed(3);
  if (typeof value === "boolean") return value ? "true" : "false";
  if (typeof value === "string") return value;
  return JSON.stringify(value);
}

function toAdminFilters(filters: Filters): AdminFilters {
  return {
    project_id: filters.projectID,
    from: filters.from,
    to: filters.to,
    event_type: filters.eventType,
    system_id: filters.systemID,
    zone_id: filters.zoneID,
    commander_id: filters.commanderID,
    game_version: filters.gameVersion,
    build_channel: filters.buildChannel,
    field_key: filters.fieldKey,
    field_value: filters.fieldValue,
    status: filters.reportStatus,
    label: filters.reportLabel,
    limit: 100,
  };
}

function tabFromURL(): Tab {
  const raw = new URLSearchParams(window.location.search).get("tab");
  return tabs.includes(raw as Tab) ? (raw as Tab) : "Overview";
}

function filtersFromURL(projectFallback: string): Filters {
  const params = new URLSearchParams(window.location.search);
  return {
    projectID: params.get("project_id") || projectFallback,
    from: params.get("from") || isoDaysAgo(30),
    to: params.get("to") || new Date().toISOString(),
    eventType: params.get("event_type") || "",
    systemID: params.get("system_id") || "",
    zoneID: params.get("zone_id") || "",
    commanderID: params.get("commander_id") || "",
    gameVersion: params.get("game_version") || "",
    buildChannel: params.get("build_channel") || "",
    fieldKey: params.get("field_key") || "",
    fieldValue: params.get("field_value") || "",
    reportStatus: params.get("status") || "",
    reportLabel: params.get("label") || "",
  };
}

function setParam(params: URLSearchParams, key: string, value: string) {
  if (value) params.set(key, value);
}

function rangeLabel(filters: Filters) {
  const from = Date.parse(filters.from);
  const to = Date.parse(filters.to);
  if (!Number.isFinite(from) || !Number.isFinite(to)) return "30d";
  const days = Math.round((to - from) / 86_400_000);
  if (filters.from.startsWith("1970-01-01")) return "all";
  const preset = timePresets.find(([, presetDays]) => presetDays === days);
  return preset?.[0] ?? "30d";
}

function applyPreset(value: string, setFilter: <K extends keyof Filters>(key: K, value: Filters[K]) => void) {
  const preset = timePresets.find(([label]) => label === value);
  if (!preset) return;
  const [, days] = preset;
  if (days === 0) {
    setFilter("from", "1970-01-01T00:00:00Z");
    setFilter("to", new Date().toISOString());
    return;
  }
  setFilter("from", isoDaysAgo(days));
  setFilter("to", new Date().toISOString());
}

function isoDaysAgo(days: number) {
  return new Date(Date.now() - days * 86_400_000).toISOString();
}

function splitLabels(value: string) {
  return value.split(",").map((label) => label.trim()).filter(Boolean);
}

function copyPermalink() {
  void copyText(window.location.href);
}

async function copyText(value: string): Promise<boolean> {
  if (!value) return false;
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value);
      return true;
    }
  } catch {
    // Fall through to the textarea fallback for local/dev browser edge cases.
  }
  return fallbackCopyText(value);
}

function fallbackCopyText(value: string): boolean {
  const textarea = document.createElement("textarea");
  textarea.value = value;
  textarea.setAttribute("readonly", "true");
  textarea.style.position = "fixed";
  textarea.style.left = "-9999px";
  textarea.style.top = "0";
  document.body.appendChild(textarea);
  textarea.focus();
  textarea.select();
  try {
    return document.execCommand("copy");
  } finally {
    document.body.removeChild(textarea);
  }
}

function exportData(filename: string, rows: unknown[], format: ExportFormat, filters: AdminFilters) {
  const payload = { exported_at: new Date().toISOString(), filters, rows };
  if (format === "json") {
    download(filename, JSON.stringify(payload, null, 2), "application/json");
    return;
  }
  if (format === "ndjson") {
    download(filename, rows.map((row) => JSON.stringify(row)).join("\n"), "application/x-ndjson");
    return;
  }
  download(filename, toCSV(rows), "text/csv");
}

function toCSV(rows: unknown[]) {
  const objects = rows.filter((row): row is Record<string, unknown> => row !== null && typeof row === "object");
  const headers = Array.from(new Set(objects.flatMap((row) => Object.keys(row))));
  return [
    headers.join(","),
    ...objects.map((row) => headers.map((header) => csvCell(row[header])).join(",")),
  ].join("\n");
}

function csvCell(value: unknown) {
  const raw = typeof value === "string" ? value : JSON.stringify(value ?? "");
  return `"${raw.replaceAll("\"", "\"\"")}"`;
}

function download(filename: string, value: string, type: string) {
  const blob = new Blob([value], { type });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  link.click();
  URL.revokeObjectURL(url);
}
