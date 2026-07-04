import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useIsMutating, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  api,
  type AdminFilters,
  type CreateProjectRequest,
  type EventSummary,
  type EventTypeSummary,
  type FunnelDefinition,
  type FunnelSummary,
  type HeatmapCell,
  type QueryField,
  type ReportDetail,
  type ReportSummary,
  type SettingsResponse,
  type TraceEvent,
} from "../api";
import {
  EventGroupsBuilder,
  FunnelsBuilder,
  QueryFieldsBuilder,
  eventGroupsFromDrafts,
  eventGroupsToDrafts,
  normalizeFunnel,
  normalizeQueryField,
  validateEventGroupDrafts,
  validateSchemaDrafts,
  type EventGroupDraft,
} from "../components/ProjectSchemaBuilders";
import { AddProjectWizard } from "../components/ProjectControls";
import { useProjectScope } from "../hooks/useProjectScope";

const tabs = [
  "Overview",
  "Events",
  "Trace",
  "Regions",
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
  regionID: string;
  zoneID: string;
  playerID: string;
  gameVersion: string;
  buildChannel: string;
  fieldKey: string;
  fieldValue: string;
  reportStatus: string;
  reportLabel: string;
}

export default function Dashboard() {
  const queryClient = useQueryClient();
  const { projectScope, setProjectScope } = useProjectScope();
  const initial = useMemo(() => filtersFromURL(projectScope || ""), []);
  const [activeTab, setActiveTab] = useState<Tab>(tabFromURL());
  const [filters, setFilters] = useState<Filters>(initial);
  const [selectedEvent, setSelectedEvent] = useState<EventSummary | null>(null);
  const [selectedReportID, setSelectedReportID] = useState<string | null>(null);
  const [addProjectOpen, setAddProjectOpen] = useState(false);
  const [emptyWizardDismissed, setEmptyWizardDismissed] = useState(false);

  const switchProject = (projectID: string) => {
    setSelectedEvent(null);
    setSelectedReportID(null);
    setFilters((prev) => ({
      ...prev,
      projectID,
      eventType: "",
      regionID: "",
      zoneID: "",
      playerID: "",
      fieldKey: "",
      fieldValue: "",
      reportStatus: "",
      reportLabel: "",
    }));
  };

  useEffect(() => {
    if (projectScope && projectScope !== filters.projectID) {
      switchProject(projectScope);
    }
  }, [filters.projectID, projectScope]);

  const projects = useQuery({
    queryKey: ["projects"],
    queryFn: () => api.projects(),
  });
  const projectList = projects.data?.projects ?? [];
  const projectsLoaded = !!projects.data;
  const noProjects = projectsLoaded && projectList.length === 0;

  useEffect(() => {
    if (!projectsLoaded) return;
    if (noProjects) {
      if (projectScope) {
        setProjectScope(null);
      }
      if (filters.projectID) {
        switchProject("");
      }
      return;
    }
    if (filters.projectID && !projectList.some((project) => project.project_id === filters.projectID)) {
      switchProject(projectList[0]?.project_id ?? "");
    }
  }, [filters.projectID, noProjects, projectList, projectScope, projectsLoaded, setProjectScope]);

  useEffect(() => {
    if (noProjects && !emptyWizardDismissed) {
      setAddProjectOpen(true);
    }
  }, [emptyWizardDismissed, noProjects]);

  useEffect(() => {
    const params = new URLSearchParams();
    params.set("tab", activeTab);
    setParam(params, "project_id", filters.projectID);
    setParam(params, "from", filters.from);
    setParam(params, "to", filters.to);
    setParam(params, "event_type", filters.eventType);
    setParam(params, "region_id", filters.regionID);
    setParam(params, "zone_id", filters.zoneID);
    setParam(params, "player_id", filters.playerID);
    setParam(params, "game_version", filters.gameVersion);
    setParam(params, "build_channel", filters.buildChannel);
    setParam(params, "field_key", filters.fieldKey);
    setParam(params, "field_value", filters.fieldValue);
    setParam(params, "status", filters.reportStatus);
    setParam(params, "label", filters.reportLabel);
    window.history.replaceState(null, "", `${window.location.pathname}?${params.toString()}`);
  }, [activeTab, filters]);

  const adminFilters = useMemo(() => toAdminFilters(filters), [filters]);
  const selectedProjectExists = projectsLoaded && projectList.some((project) => project.project_id === filters.projectID);
  const hasProject = !!filters.projectID && selectedProjectExists;

  const summary = useQuery({
    enabled: hasProject,
    queryKey: ["summary", adminFilters],
    queryFn: () => api.summary(adminFilters),
  });
  const events = useQuery({
    enabled: hasProject,
    queryKey: ["events", adminFilters],
    queryFn: () => api.events(adminFilters),
  });
  const reports = useQuery({
    enabled: hasProject,
    queryKey: ["reports", adminFilters],
    queryFn: () => api.reports(adminFilters),
  });
  const eventTypes = useQuery({
    enabled: hasProject,
    queryKey: ["event-types", filters.projectID],
    queryFn: () => api.eventTypes(filters.projectID),
  });
  const settings = useQuery({
    enabled: hasProject,
    queryKey: ["settings", filters.projectID],
    queryFn: () => api.settings(filters.projectID),
  });
  const spatialEnabled = settings.data?.project.map_config.spatial_enabled !== false;
  const regionHeatmap = useQuery({
    enabled: hasProject && spatialEnabled,
    queryKey: ["region-heatmap", adminFilters],
    queryFn: () => api.regionHeatmap(adminFilters),
  });
  const firstRegion = filters.regionID || regionHeatmap.data?.cells[0]?.region_id || "unknown";
  const zoneHeatmap = useQuery({
    enabled: hasProject && spatialEnabled,
    queryKey: ["zone-heatmap", adminFilters, firstRegion],
    queryFn: () => api.zoneHeatmap({ ...adminFilters, region_id: firstRegion }),
  });
  const funnels = useQuery({
    enabled: hasProject,
    queryKey: ["funnels", adminFilters],
    queryFn: () => api.funnels(adminFilters),
  });

  const selectedPlayer = filters.playerID || events.data?.events[0]?.player_id || "";
  const trace = useQuery({
    enabled: hasProject && !!selectedPlayer,
    queryKey: ["trace", filters.projectID, selectedPlayer],
    queryFn: () =>
      selectedPlayer
        ? api.playerTrace(filters.projectID, selectedPlayer)
        : Promise.resolve({ events: [] }),
  });

  const selectedReport = useQuery({
    enabled: hasProject && !!selectedReportID,
    queryKey: ["report-detail", filters.projectID, selectedReportID],
    queryFn: () => api.reportDetail(filters.projectID, selectedReportID || ""),
  });

  const currentRows = useMemo(() => {
    if (activeTab === "Reports") return reports.data?.reports ?? [];
    if (activeTab === "Schema") return eventTypes.data?.event_types ?? [];
    if (activeTab === "Funnels") return funnels.data?.funnels ?? [];
    if (activeTab === "Regions") return regionHeatmap.data?.cells ?? [];
    if (activeTab === "Zone") return zoneHeatmap.data?.cells ?? [];
    if (activeTab === "Trace") return trace.data?.events ?? [];
    return events.data?.events ?? [];
  }, [activeTab, eventTypes.data, events.data, funnels.data, reports.data, regionHeatmap.data, trace.data, zoneHeatmap.data]);

  const exportRows = (format: ExportFormat) => {
    exportData(`${activeTab.toLowerCase()}-${filters.projectID}.${format}`, currentRows, format, adminFilters);
  };

  const setFilter = <K extends keyof Filters>(key: K, value: Filters[K]) => {
    setFilters((prev) => ({ ...prev, [key]: value }));
  };
  const projectName = settings.data?.project.display_name || filters.projectID;
  const spatialTabs: Tab[] = ["Regions", "Zone"];
  const visibleTabs = tabs.filter((tab) => spatialEnabled || !spatialTabs.includes(tab));

  if (noProjects) {
    return (
      <div className="space-y-5">
        <ProjectEmptyState onAddProject={() => setAddProjectOpen(true)} />
        {addProjectOpen ? (
          <AddProjectWizard
            onClose={() => {
              setAddProjectOpen(false);
              setEmptyWizardDismissed(true);
            }}
            onCreated={(project) => {
              queryClient.invalidateQueries({ queryKey: ["projects"] });
              queryClient.invalidateQueries({ queryKey: ["settings", project.project_id] });
              setProjectScope(project.project_id);
              switchProject(project.project_id);
              setAddProjectOpen(false);
              setEmptyWizardDismissed(false);
            }}
          />
        ) : null}
      </div>
    );
  }

  return (
    <div className="space-y-5">
      <header className="flex flex-wrap items-center gap-3">
        <div>
          <p className="text-sm uppercase tracking-wide text-on-surface-variant">Collector Console</p>
          <h1 className="text-3xl font-bold text-on-surface">{projectName}</h1>
        </div>
        <div className="ml-auto flex flex-wrap items-end gap-2">
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
        {visibleTabs.map((tab) => (
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
          onTrace={(playerID) => {
            setFilter("playerID", playerID);
            setActiveTab("Trace");
          }}
        />
      ) : null}
      {activeTab === "Trace" ? (
        <TraceTable
          playerID={selectedPlayer}
          events={trace.data?.events ?? []}
          queryFields={settings.data?.project.query_fields ?? []}
        />
      ) : null}
      {activeTab === "Regions" ? (
        <HeatmapTable
          cells={regionHeatmap.data?.cells ?? []}
          onSelect={(cell) => {
            setFilter("regionID", cell.region_id);
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
          onTrace={(playerID) => {
            setFilter("playerID", playerID);
            setActiveTab("Trace");
          }}
        />
      ) : null}
      {activeTab === "Schema" ? (
        <SchemaTable
          eventTypes={eventTypes.data?.event_types ?? []}
          settings={settings.data}
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
        onTrace={(playerID) => {
          setFilter("playerID", playerID);
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
        <Input label="Region" value={filters.regionID} onChange={(value) => setFilter("regionID", value)} />
        <Input label="Zone" value={filters.zoneID} onChange={(value) => setFilter("zoneID", value)} />
        <Input label="Player" value={filters.playerID} onChange={(value) => setFilter("playerID", value)} />
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

function ProjectEmptyState({ onAddProject }: { onAddProject: () => void }) {
  return (
    <section className="rounded-md border border-outline-ghost bg-surface-container-low p-6">
      <div className="max-w-2xl space-y-3">
        <p className="text-sm uppercase tracking-wide text-on-surface-variant">Collector Console</p>
        <h1 className="text-2xl font-bold text-on-surface">No projects configured</h1>
        <p className="text-sm text-on-surface-variant">
          Add a project to define its ingest defaults, query fields, event groups, and funnels.
        </p>
        <button type="button" onClick={onAddProject} className="btn-primary">
          Add Project
        </button>
      </div>
    </section>
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
    ["Players", summary?.player_count ?? 0],
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
  onTrace: (playerID: string) => void;
}) {
  const visibleFields = queryFields.slice(0, 4);
  return (
    <Table
      headers={["Type", "Player", "Region", "Zone", ...visibleFields.map((field) => field.label || field.key), "Version", "Time", "Actions"]}
      rows={events.map((event) => [
        event.event_type,
        event.player_id,
        event.region_id,
        event.zone_id,
        ...visibleFields.map((field) => formatFieldValue(event.fields?.[field.key])),
        event.game_version,
        event.real_ts,
        <div className="flex gap-2">
          <button type="button" onClick={() => onOpen(event)} className="link-button">Open</button>
          <button type="button" onClick={() => onTrace(event.player_id)} className="link-button">Trace</button>
          <button type="button" onClick={() => copyText(event.id)} className="link-button">Copy ID</button>
        </div>,
      ])}
    />
  );
}

function TraceTable({ playerID, events, queryFields }: { playerID?: string; events: TraceEvent[]; queryFields?: QueryField[] }) {
  const visibleFields = (queryFields ?? []).slice(0, 3);
  return (
    <section className="space-y-3">
      <div className="flex items-center gap-2 text-sm text-on-surface-variant">
        <span>{playerID || "No player selected"}</span>
        {playerID ? <button type="button" onClick={() => copyText(playerID)} className="link-button">Copy Player</button> : null}
      </div>
      <Table
        headers={["Type", "Region", "Zone", ...visibleFields.map((field) => field.label || field.key), "Game time", "Time", "Payload"]}
        rows={events.map((event) => [
          event.event_type,
          event.region_id,
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
      headers={["Region", "Zone", "Grid", "Type", "Count", "Actions"]}
      rows={cells.map((cell) => [
        cell.region_id,
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
      headers={["Name", "Description", "Started", "Completed", "Rate", "Drop-off", "Steps"]}
      rows={funnels.map((funnel) => [
        funnel.name,
        funnel.description,
        String(funnel.started),
        String(funnel.completed),
        `${Math.round(funnel.rate * 100)}%`,
        funnel.dropoff,
        (funnel.steps ?? []).map((step) => `${step.label}: ${step.count}`).join(", "),
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
  onTrace: (playerID: string) => void;
}) {
  return (
    <Table
      headers={["Details", "Report", "Status", "Labels", "Mood", "Notes", "Player", "Region", "Created", "Trace"]}
      rows={reports.map((report) => [
        <button type="button" onClick={() => onOpen(report)} className="btn-secondary">Open</button>,
        <button type="button" onClick={() => onOpen(report)} className="link-button">{report.report_id}</button>,
        report.status,
        report.labels.join(", "),
        report.mood_label,
        report.notes_preview,
        report.player_id,
        report.region_id,
        report.created_at,
        <button type="button" onClick={() => onTrace(report.player_id)} className="link-button">Trace</button>,
      ])}
    />
  );
}

function SchemaTable({
  eventTypes,
  settings,
}: {
  eventTypes: EventTypeSummary[];
  settings?: SettingsResponse;
}) {
  const queryClient = useQueryClient();
  const project = settings?.project;
  const [eventGroups, setEventGroups] = useState<EventGroupDraft[]>([]);
  const [queryFields, setQueryFields] = useState<QueryField[]>([]);
  const [funnels, setFunnels] = useState<FunnelDefinition[]>([]);
  const [validationError, setValidationError] = useState("");
  const [saveStatus, setSaveStatus] = useState("");
  const updateProjectMutationKey = ["update-project", project?.project_id ?? ""] as const;
  const projectUpdatePending = useIsMutating({ mutationKey: updateProjectMutationKey }) > 0;

  useEffect(() => {
    if (!project) return;
    setEventGroups(eventGroupsToDrafts(project.event_groups ?? {}));
    setQueryFields(project.query_fields ?? []);
    setFunnels(project.funnels ?? []);
    setValidationError("");
    setSaveStatus("");
  }, [project?.project_id]);

  const updateProject = useMutation({
    mutationKey: updateProjectMutationKey,
    mutationFn: (body: CreateProjectRequest) => api.updateProject(body),
    onSuccess: (updatedProject) => {
      setSaveStatus("Saved");
      queryClient.setQueryData<SettingsResponse>(["settings", updatedProject.project_id], (current) =>
        current ? { ...current, project: updatedProject } : current,
      );
      queryClient.invalidateQueries({ queryKey: ["settings", updatedProject.project_id] });
      queryClient.invalidateQueries({ queryKey: ["projects"] });
      queryClient.invalidateQueries({ queryKey: ["funnels"] });
      queryClient.invalidateQueries({ queryKey: ["event-types", updatedProject.project_id] });
    },
  });

  const submit = () => {
    if (!project || projectUpdatePending) return;
    setValidationError("");
    setSaveStatus("");
    const eventGroupError = validateEventGroupDrafts(eventGroups);
    if (eventGroupError) {
      setValidationError(eventGroupError);
      return;
    }
    const preparedEventGroups = eventGroupsFromDrafts(eventGroups);
    const preparedQueryFields = queryFields.map(normalizeQueryField).filter((field) => field.key || field.source || field.label);
    const preparedFunnels = funnels.map(normalizeFunnel).filter((funnel) => funnel.id || funnel.name || funnel.steps.length > 0);
    const schemaError = validateSchemaDrafts(preparedQueryFields, preparedFunnels);
    if (schemaError) {
      setValidationError(schemaError);
      return;
    }
    const latestProject = queryClient.getQueryData<SettingsResponse>(["settings", project.project_id])?.project ?? project;
    updateProject.mutate(projectUpdateRequest(latestProject, {
      event_groups: preparedEventGroups,
      query_fields: preparedQueryFields,
      funnels: preparedFunnels,
    }));
  };

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
      {project ? (
        <div className="space-y-4">
          <div className="grid gap-4 xl:grid-cols-3">
            <EventGroupsBuilder groups={eventGroups} onChange={setEventGroups} />
            <QueryFieldsBuilder fields={queryFields} onChange={setQueryFields} />
            <FunnelsBuilder funnels={funnels} queryFields={queryFields} onChange={setFunnels} />
          </div>
          {validationError || updateProject.error ? (
            <p className="text-sm text-status-error">
              {validationError || (updateProject.error instanceof Error ? updateProject.error.message : "Failed to save schema")}
            </p>
          ) : null}
          {saveStatus ? <p className="text-sm text-on-surface-variant">{saveStatus}</p> : null}
          <div className="flex justify-end">
            <button type="button" onClick={submit} disabled={projectUpdatePending} className="btn-primary disabled:opacity-50">
              {projectUpdatePending ? "Saving..." : "Save Schema"}
            </button>
          </div>
        </div>
      ) : (
        <Panel>
          <p className="text-sm text-on-surface-variant">Loading schema settings...</p>
        </Panel>
      )}
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
          <Info label="Session" value="Signed admin cookie" />
          <Info label="Admin access" value="Google OAuth, domain gate, invitations" />
          <Info label="Local screenshots" value="REPORT_STORAGE_BACKEND=local, REPORT_STORAGE_DIR=var/reports" />
          <Info label="R2 screenshots" value="REPORT_STORAGE_BACKEND=r2 with R2_ENDPOINT, R2_BUCKET, R2_ACCESS_KEY_ID" />
        </dl>
      </Panel>
      <ProjectSettingsEditor settings={settings} />
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
          <Input label="Token name" value={tokenName} onChange={setTokenName} placeholder="local-dev" />
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
    </div>
  );
}

function ProjectSettingsEditor({ settings }: { settings?: SettingsResponse }) {
  const queryClient = useQueryClient();
  const project = settings?.project;
  const [displayName, setDisplayName] = useState("");
  const [validationMode, setValidationMode] = useState<"warn" | "strict">("warn");
  const [maxEvents, setMaxEvents] = useState("");
  const [allowUnknownEvents, setAllowUnknownEvents] = useState(true);
  const [allowScreenshotFailures, setAllowScreenshotFailures] = useState(true);
  const [eventDays, setEventDays] = useState("");
  const [reportDays, setReportDays] = useState("");
  const [accessLogDays, setAccessLogDays] = useState("");
  const [spatialEnabled, setSpatialEnabled] = useState(true);
  const [zoneExtentM, setZoneExtentM] = useState("");
  const [zoneCellM, setZoneCellM] = useState("");
  const [reportStatusesValue, setReportStatusesValue] = useState("");
  const [reportLabelsValue, setReportLabelsValue] = useState("");
  const [rateLimitSeconds, setRateLimitSeconds] = useState("");
  const [validationError, setValidationError] = useState("");
  const [saveStatus, setSaveStatus] = useState("");
  const updateProjectMutationKey = ["update-project", project?.project_id ?? ""] as const;
  const projectUpdatePending = useIsMutating({ mutationKey: updateProjectMutationKey }) > 0;

  useEffect(() => {
    if (!project) return;
    setDisplayName(project.display_name ?? "");
    setValidationMode(project.validation_mode === "strict" ? "strict" : "warn");
    setMaxEvents(configNumber(project.ingest_config, "max_events_per_batch"));
    setAllowUnknownEvents(configBool(project.ingest_config, "allow_unknown_event_types", true));
    setAllowScreenshotFailures(configBool(project.ingest_config, "allow_screenshot_failures", true));
    setEventDays(configNumber(project.retention_config, "event_days"));
    setReportDays(configNumber(project.retention_config, "report_days"));
    setAccessLogDays(configNumber(project.retention_config, "access_log_days"));
    setSpatialEnabled(configBool(project.map_config, "spatial_enabled", true));
    setZoneExtentM(configNumber(project.map_config, "zone_extent_m"));
    setZoneCellM(configNumber(project.map_config, "zone_heatmap_cell_m"));
    setReportStatusesValue(configStringList(project.report_config, "statuses"));
    setReportLabelsValue(configStringList(project.report_config, "labels"));
    setRateLimitSeconds(configNumber(project.report_config, "rate_limit_seconds"));
    setValidationError("");
    setSaveStatus("");
  }, [project?.project_id]);

  const updateProject = useMutation({
    mutationKey: updateProjectMutationKey,
    mutationFn: (body: CreateProjectRequest) => api.updateProject(body),
    onSuccess: (updatedProject) => {
      setSaveStatus("Saved");
      queryClient.setQueryData<SettingsResponse>(["settings", updatedProject.project_id], (current) =>
        current ? { ...current, project: updatedProject } : current,
      );
      queryClient.invalidateQueries({ queryKey: ["settings", updatedProject.project_id] });
      queryClient.invalidateQueries({ queryKey: ["projects"] });
    },
  });

  const submit = () => {
    if (!project || projectUpdatePending) return;
    setValidationError("");
    setSaveStatus("");
    const trimmedDisplayName = displayName.trim();
    if (!trimmedDisplayName) {
      setValidationError("Display name is required.");
      return;
    }
    const latestProject = queryClient.getQueryData<SettingsResponse>(["settings", project.project_id])?.project ?? project;
    updateProject.mutate(projectUpdateRequest(latestProject, {
      display_name: trimmedDisplayName,
      validation_mode: validationMode,
      ingest_config: {
        ...latestProject.ingest_config,
        max_events_per_batch: parsePositiveInt(maxEvents, 50),
        accept_gzip: configBool(latestProject.ingest_config, "accept_gzip", true),
        allow_unknown_event_types: allowUnknownEvents,
        allow_screenshot_failures: allowScreenshotFailures,
      },
      retention_config: {
        ...latestProject.retention_config,
        event_days: parsePositiveInt(eventDays, 730),
        report_days: parsePositiveInt(reportDays, 1095),
        access_log_days: parsePositiveInt(accessLogDays, 14),
      },
      map_config: {
        ...latestProject.map_config,
        spatial_enabled: spatialEnabled,
        zone_extent_m: parsePositiveInt(zoneExtentM, 30000),
        zone_heatmap_cell_m: parsePositiveInt(zoneCellM, 300),
      },
      report_config: {
        ...latestProject.report_config,
        statuses: splitLabels(reportStatusesValue),
        labels: splitLabels(reportLabelsValue),
        rate_limit_seconds: parsePositiveInt(rateLimitSeconds, 60),
      },
    }));
  };

  if (!project) {
    return (
      <Panel>
        <p className="text-sm text-on-surface-variant">Loading project settings...</p>
      </Panel>
    );
  }

  return (
    <Panel>
      <div className="space-y-4">
        <div className="grid gap-3 md:grid-cols-2">
          <Input label="Display name" value={displayName} onChange={setDisplayName} />
          <Select label="Validation" value={validationMode} options={["warn", "strict"]} onChange={(value) => setValidationMode(value as "warn" | "strict")} />
        </div>
        <div className="grid gap-4 lg:grid-cols-2">
          <section className="grid gap-3 md:grid-cols-2">
            <Input label="Max events per batch" value={maxEvents} onChange={setMaxEvents} />
            <Input label="Bug report rate limit" value={rateLimitSeconds} onChange={setRateLimitSeconds} />
            <Checkbox label="Allow unknown event types" checked={allowUnknownEvents} onChange={setAllowUnknownEvents} />
            <Checkbox label="Allow screenshot failures" checked={allowScreenshotFailures} onChange={setAllowScreenshotFailures} />
            <Input label="Event retention days" value={eventDays} onChange={setEventDays} />
            <Input label="Report retention days" value={reportDays} onChange={setReportDays} />
            <Input label="Access log retention days" value={accessLogDays} onChange={setAccessLogDays} />
          </section>
          <section className="grid gap-3">
            <Checkbox label="Spatial maps enabled" checked={spatialEnabled} onChange={setSpatialEnabled} />
            <div className="grid gap-3 md:grid-cols-2">
              <Input label="Zone extent m" value={zoneExtentM} onChange={setZoneExtentM} />
              <Input label="Heatmap cell m" value={zoneCellM} onChange={setZoneCellM} />
            </div>
            <Input label="Report statuses" value={reportStatusesValue} onChange={setReportStatusesValue} />
            <Input label="Report labels" value={reportLabelsValue} onChange={setReportLabelsValue} />
          </section>
        </div>
        {validationError || updateProject.error ? (
          <p className="text-sm text-status-error">
            {validationError || (updateProject.error instanceof Error ? updateProject.error.message : "Failed to save settings")}
          </p>
        ) : null}
        {saveStatus ? <p className="text-sm text-on-surface-variant">{saveStatus}</p> : null}
        <div className="flex justify-end">
          <button type="button" onClick={submit} disabled={projectUpdatePending} className="btn-primary disabled:opacity-50">
            {projectUpdatePending ? "Saving..." : "Save Settings"}
          </button>
        </div>
      </div>
    </Panel>
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
            ["Player", event.player_id],
            ["Region", event.region_id],
            ["Zone", event.zone_id],
            ["Version", event.game_version],
            ["Channel", event.build_channel],
            ["Commit", event.commit_sha || "-"],
          ]}
        />
        {fieldRows.length > 0 ? <InfoGrid rows={fieldRows} /> : null}
        <div className="flex flex-wrap gap-2">
          <button type="button" onClick={() => copyText(event.id)} className="btn-secondary">Copy Event ID</button>
          <button type="button" onClick={() => copyText(event.player_id)} className="btn-secondary">Copy Player</button>
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
  onTrace: (playerID: string) => void;
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
              ["Player", report.player_id],
              ["Region", report.region_id],
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
            <button type="button" onClick={() => onTrace(report.player_id)} className="btn-secondary">Trace Player</button>
            <button type="button" onClick={() => copyText(report.player_id)} className="btn-secondary">Copy Player</button>
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
          <TraceTable playerID={report.player_id} events={report.trace} />
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

function Checkbox({ label, checked, onChange }: { label: string; checked: boolean; onChange: (checked: boolean) => void }) {
  return (
    <label className="flex items-center gap-2 text-sm text-on-surface-variant">
      <input
        type="checkbox"
        checked={checked}
        onChange={(event) => onChange(event.target.checked)}
        className="h-4 w-4 rounded border-outline-ghost bg-surface-container accent-primary"
      />
      <span>{label}</span>
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
    region_id: filters.regionID,
    zone_id: filters.zoneID,
    player_id: filters.playerID,
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
    regionID: params.get("region_id") || "",
    zoneID: params.get("zone_id") || "",
    playerID: params.get("player_id") || "",
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

function projectUpdateRequest(
  project: SettingsResponse["project"],
  patch: Partial<CreateProjectRequest>,
): CreateProjectRequest {
  return {
    project_id: project.project_id,
    display_name: project.display_name,
    validation_mode: project.validation_mode === "strict" ? "strict" : "warn",
    ingest_config: project.ingest_config,
    retention_config: project.retention_config,
    map_config: project.map_config,
    report_config: project.report_config,
    event_groups: project.event_groups ?? {},
    query_fields: project.query_fields ?? [],
    funnels: project.funnels ?? [],
    ...patch,
  };
}

function configNumber(config: Record<string, unknown>, key: string): string {
  const value = config[key];
  return typeof value === "number" ? String(value) : "";
}

function configBool(config: Record<string, unknown>, key: string, fallback: boolean): boolean {
  const value = config[key];
  return typeof value === "boolean" ? value : fallback;
}

function configStringList(config: Record<string, unknown>, key: string): string {
  const value = config[key];
  if (!Array.isArray(value)) return "";
  return value.filter((item): item is string => typeof item === "string").join(", ");
}

function parsePositiveInt(value: string, fallback: number) {
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
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
