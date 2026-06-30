import { useEffect, useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  api,
  type CreateProjectRequest,
  type ProjectSummary,
  type QueryField,
} from "../api";
import { useProjectScope } from "../hooks/useProjectScope";

const reportStatuses = ["new", "seen", "needs_more_info", "reproduced", "fixed", "wont_fix"];

export default function ProjectControls({className}: {className?: string}) {
  const queryClient = useQueryClient();
  const { projectScope, setProjectScope } = useProjectScope();
  const [addProjectOpen, setAddProjectOpen] = useState(false);
  const projects = useQuery({
    queryKey: ["projects"],
    queryFn: () => api.projects(),
  });
  const projectID = projectScope || "";

  useEffect(() => {
    if (!projectScope && projects.data?.projects[0]?.project_id) {
      setProjectScope(projects.data.projects[0].project_id);
    }
  }, [projectScope, projects.data?.projects, setProjectScope]);

  return (
    <>
      <nav className={className}>
        <ol className="m-0 flex list-none items-center gap-8 p-0">
          <li>
            <ProjectSwitcher
              projectID={projectID}
              projects={projects.data?.projects ?? []}
              onChange={(nextProjectID) => setProjectScope(nextProjectID)}
            />
          </li>

          <li>
            <button type="button" onClick={() => setAddProjectOpen(true)} className="btn-primary">
              Add Project
            </button>
          </li>
        </ol>
      </nav>
      {addProjectOpen ? (
        <AddProjectWizard
          onClose={() => setAddProjectOpen(false)}
          onCreated={(project) => {
            queryClient.invalidateQueries({ queryKey: ["projects"] });
            queryClient.invalidateQueries({ queryKey: ["settings", project.project_id] });
            setProjectScope(project.project_id);
            setAddProjectOpen(false);
          }}
        />
      ) : null}
    </>
  );
}

function ProjectSwitcher({
  projectID,
  projects,
  onChange,
}: {
  projectID: string;
  projects: ProjectSummary[];
  onChange: (projectID: string) => void;
}) {
  const hasCurrent = projects.some((project) => project.project_id === projectID);
  const options = !projectID || hasCurrent
    ? projects
    : [{ project_id: projectID, display_name: projectID, validation_mode: "", created_at: "", updated_at: "" }, ...projects];
  return (
    <select
      value={projectID}
      onChange={(event) => onChange(event.target.value)}
      className="block min-w-56 text-sm mt-1 w-full rounded-md border border-outline-ghost bg-surface-container px-2 py-1 text-on-surface outline-none focus:border-primary"
    >
      {!projectID ? <option value="">Select project</option> : null}
      {options.map((project) => (
        <option key={project.project_id} value={project.project_id}>
          {project.display_name} ({project.project_id})
        </option>
      ))}
    </select>
  );
}

function AddProjectWizard({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: (project: { project_id: string }) => void;
}) {
  const [step, setStep] = useState(0);
  const [projectID, setProjectID] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [validationMode, setValidationMode] = useState<"warn" | "strict">("warn");
  const [maxEvents, setMaxEvents] = useState("50");
  const [allowUnknownEvents, setAllowUnknownEvents] = useState(true);
  const [allowScreenshotFailures, setAllowScreenshotFailures] = useState(true);
  const [eventDays, setEventDays] = useState("730");
  const [reportDays, setReportDays] = useState("1095");
  const [accessLogDays, setAccessLogDays] = useState("14");
  const [spatialEnabled, setSpatialEnabled] = useState(true);
  const [zoneExtentM, setZoneExtentM] = useState("30000");
  const [zoneCellM, setZoneCellM] = useState("300");
  const [reportStatusesValue, setReportStatusesValue] = useState(reportStatuses.join(", "));
  const [reportLabelsValue, setReportLabelsValue] = useState("bug, sentiment, balance, mission, combat, economy, ui");
  const [rateLimitSeconds, setRateLimitSeconds] = useState("60");
  const [eventGroupsValue, setEventGroupsValue] = useState(JSON.stringify(defaultEventGroups, null, 2));
  const [queryFieldsValue, setQueryFieldsValue] = useState(JSON.stringify(defaultQueryFields, null, 2));
  const [validationError, setValidationError] = useState("");

  const createProject = useMutation({
    mutationFn: (body: CreateProjectRequest) => api.createProject(body),
    onSuccess: (project) => onCreated(project),
  });

  const buildRequest = (): CreateProjectRequest | null => {
    setValidationError("");
    const trimmedProjectID = projectID.trim();
    const trimmedDisplayName = displayName.trim();
    if (!trimmedProjectID || !trimmedDisplayName) {
      setValidationError("Project ID and display name are required.");
      return null;
    }
    let eventGroups: Record<string, string[]>;
    let queryFields: QueryField[];
    try {
      eventGroups = JSON.parse(eventGroupsValue);
      queryFields = JSON.parse(queryFieldsValue);
    } catch (err) {
      setValidationError(err instanceof Error ? err.message : "Schema JSON is invalid.");
      return null;
    }
    return {
      project_id: trimmedProjectID,
      display_name: trimmedDisplayName,
      validation_mode: validationMode,
      ingest_config: {
        max_events_per_batch: parsePositiveInt(maxEvents, 50),
        accept_gzip: true,
        allow_unknown_event_types: allowUnknownEvents,
        allow_screenshot_failures: allowScreenshotFailures,
      },
      retention_config: {
        event_days: parsePositiveInt(eventDays, 730),
        report_days: parsePositiveInt(reportDays, 1095),
        access_log_days: parsePositiveInt(accessLogDays, 14),
      },
      map_config: {
        spatial_enabled: spatialEnabled,
        zone_extent_m: parsePositiveInt(zoneExtentM, 30000),
        zone_heatmap_cell_m: parsePositiveInt(zoneCellM, 300),
      },
      report_config: {
        statuses: splitLabels(reportStatusesValue),
        labels: splitLabels(reportLabelsValue),
        rate_limit_seconds: parsePositiveInt(rateLimitSeconds, 60),
      },
      event_groups: eventGroups,
      query_fields: queryFields,
    };
  };

  const submit = () => {
    const body = buildRequest();
    if (body && !createProject.isPending) {
      createProject.mutate(body);
    }
  };

  return (
    <Drawer title="Add Project" onClose={onClose}>
      <div className="space-y-4 text-sm">
        <div className="flex flex-wrap gap-2">
          {["Identity", "Defaults", "Schema"].map((label, index) => (
            <button
              key={label}
              type="button"
              onClick={() => setStep(index)}
              className={step === index ? "btn-primary" : "btn-secondary"}
            >
              {label}
            </button>
          ))}
        </div>

        {step === 0 ? (
          <div className="grid gap-3 md:grid-cols-2">
            <Input label="Project ID" value={projectID} onChange={(value) => setProjectID(slugProjectID(value))} placeholder="my-game" />
            <Input label="Display name" value={displayName} onChange={setDisplayName} placeholder="My Game" />
            <Select label="Validation" value={validationMode} options={["warn", "strict"]} onChange={(value) => setValidationMode(value as "warn" | "strict")} />
          </div>
        ) : null}

        {step === 1 ? (
          <div className="grid gap-4 lg:grid-cols-2">
            <Panel>
              <div className="grid gap-3 md:grid-cols-2">
                <Input label="Max events per batch" value={maxEvents} onChange={setMaxEvents} />
                <Input label="Bug report rate limit" value={rateLimitSeconds} onChange={setRateLimitSeconds} />
                <Checkbox label="Allow unknown event types" checked={allowUnknownEvents} onChange={setAllowUnknownEvents} />
                <Checkbox label="Allow screenshot failures" checked={allowScreenshotFailures} onChange={setAllowScreenshotFailures} />
                <Input label="Event retention days" value={eventDays} onChange={setEventDays} />
                <Input label="Report retention days" value={reportDays} onChange={setReportDays} />
                <Input label="Access log retention days" value={accessLogDays} onChange={setAccessLogDays} />
              </div>
            </Panel>
            <Panel>
              <div className="grid gap-3">
                <Checkbox label="Spatial maps enabled" checked={spatialEnabled} onChange={setSpatialEnabled} />
                <div className="grid gap-3 md:grid-cols-2">
                  <Input label="Zone extent m" value={zoneExtentM} onChange={setZoneExtentM} />
                  <Input label="Heatmap cell m" value={zoneCellM} onChange={setZoneCellM} />
                </div>
                <Input label="Report statuses" value={reportStatusesValue} onChange={setReportStatusesValue} />
                <Input label="Report labels" value={reportLabelsValue} onChange={setReportLabelsValue} />
              </div>
            </Panel>
          </div>
        ) : null}

        {step === 2 ? (
          <div className="grid gap-3 lg:grid-cols-2">
            <TextArea label="Event groups JSON" value={eventGroupsValue} onChange={setEventGroupsValue} rows={18} />
            <TextArea label="Query fields JSON" value={queryFieldsValue} onChange={setQueryFieldsValue} rows={18} />
          </div>
        ) : null}

        {validationError || createProject.error ? (
          <p className="text-sm text-status-error">
            {validationError || (createProject.error instanceof Error ? createProject.error.message : "Failed to save project")}
          </p>
        ) : null}

        <div className="flex flex-wrap justify-end gap-2">
          <button type="button" onClick={onClose} className="btn-secondary">Cancel</button>
          {step > 0 ? <button type="button" onClick={() => setStep(step - 1)} className="btn-secondary">Back</button> : null}
          {step < 2 ? (
            <button type="button" onClick={() => setStep(step + 1)} className="btn-primary">Next</button>
          ) : (
            <button type="button" onClick={submit} disabled={createProject.isPending} className="btn-primary disabled:opacity-50">
              {createProject.isPending ? "Saving..." : "Save Project"}
            </button>
          )}
        </div>
      </div>
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

function Input({ label, value, onChange, placeholder }: { label: string; value: string; onChange: (value: string) => void; placeholder?: string }) {
  return (
    <label className="block text-sm text-on-surface-variant">
      {label}
      <input value={value} placeholder={placeholder} onChange={(event) => onChange(event.target.value)} className="mt-1 w-full rounded-md border border-outline-ghost bg-surface-container px-2 py-1 text-on-surface outline-none focus:border-primary" />
    </label>
  );
}

function TextArea({
  label,
  value,
  onChange,
  rows,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  rows?: number;
}) {
  return (
    <label className="block text-sm text-on-surface-variant">
      {label}
      <textarea
        value={value}
        rows={rows ?? 8}
        onChange={(event) => onChange(event.target.value)}
        className="mt-1 w-full rounded-md border border-outline-ghost bg-surface-container px-2 py-1 font-mono text-xs text-on-surface outline-none focus:border-primary"
      />
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

function slugProjectID(value: string) {
  return value.trim().toLowerCase().replace(/[^a-z0-9_-]/g, "-");
}

function parsePositiveInt(value: string, fallback: number) {
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
}

function splitLabels(value: string) {
  return value.split(",").map((label) => label.trim()).filter(Boolean);
}

const defaultEventGroups: Record<string, string[]> = {
  lifecycle: ["new_game", "game_continue", "game_exit", "dock", "undock"],
  economy: ["buy_commodity", "sell_commodity", "buy_intel", "sell_intel", "purchase_ship", "change_equipment", "clear_bounty"],
  mission: ["take_mission", "abandon_mission", "complete_mission", "complete_mission_objective", "mission_complication"],
  combat: ["player_death", "player_kills_npc", "npc_enters_combat_with_player", "player_enters_combat_with_npc"],
  legal: ["receive_bounty", "faction_rep_change"],
  report: ["bug_report"],
};

const defaultQueryFields: QueryField[] = [
  {
    key: "economy.credits",
    source: "metrics.economy.credits",
    type: "number",
    label: "Credits",
    filterable: true,
    aggregations: ["min", "max", "avg"],
  },
  {
    key: "ship.hull_pct",
    source: "metrics.ship.hull_pct",
    type: "number",
    label: "Hull",
    filterable: true,
    aggregations: ["min", "avg", "histogram"],
  },
  {
    key: "ship.shield_pct",
    source: "metrics.ship.shield_pct",
    type: "number",
    label: "Shield",
    filterable: true,
    aggregations: ["min", "avg", "histogram"],
  },
  {
    key: "ship.id",
    source: "dimensions.ship.id",
    type: "string",
    label: "Ship",
    filterable: true,
    aggregations: ["count"],
  },
];
