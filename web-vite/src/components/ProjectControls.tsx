import { useEffect, useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  api,
  type CreateProjectRequest,
  type FunnelDefinition,
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
  const [eventGroups, setEventGroups] = useState<EventGroupDraft[]>(() => eventGroupsToDrafts(defaultEventGroups));
  const [queryFields, setQueryFields] = useState<QueryField[]>(defaultQueryFields);
  const [funnels, setFunnels] = useState<FunnelDefinition[]>(defaultFunnels);
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
    const eventGroupError = validateEventGroupDrafts(eventGroups);
    if (eventGroupError) {
      setValidationError(eventGroupError);
      return null;
    }
    const preparedEventGroups = eventGroupsFromDrafts(eventGroups);
    const preparedQueryFields = queryFields.map(normalizeQueryField).filter((field) => field.key || field.source || field.label);
    const preparedFunnels = funnels.map(normalizeFunnel).filter((funnel) => funnel.id || funnel.name || funnel.steps.length > 0);
    const schemaError = validateSchemaDrafts(preparedQueryFields, preparedFunnels);
    if (schemaError) {
      setValidationError(schemaError);
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
      event_groups: preparedEventGroups,
      query_fields: preparedQueryFields,
      funnels: preparedFunnels,
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
          <div className="grid gap-4 xl:grid-cols-3">
            <EventGroupsBuilder groups={eventGroups} onChange={setEventGroups} />
            <QueryFieldsBuilder fields={queryFields} onChange={setQueryFields} />
            <FunnelsBuilder funnels={funnels} queryFields={queryFields} onChange={setFunnels} />
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

type EventGroupDraft = {
  name: string;
  events: string;
};

function EventGroupsBuilder({
  groups,
  onChange,
}: {
  groups: EventGroupDraft[];
  onChange: (groups: EventGroupDraft[]) => void;
}) {
  const update = (index: number, patch: Partial<EventGroupDraft>) => {
    onChange(groups.map((group, current) => current === index ? { ...group, ...patch } : group));
  };
  const remove = (index: number) => onChange(groups.filter((_, current) => current !== index));
  return (
    <Panel>
      <div className="mb-3 flex items-center gap-2">
        <h3 className="font-semibold text-on-surface">Event groups</h3>
        <button type="button" onClick={() => onChange([...groups, { name: "", events: "" }])} className="ml-auto btn-secondary">Add</button>
      </div>
      <div className="space-y-3">
        {groups.map((group, index) => (
          <div key={index} className="rounded-md border border-outline-ghost bg-surface-container p-3">
            <div className="grid gap-2">
              <Input label="Group" value={group.name} onChange={(value) => update(index, { name: slugConfigKey(value) })} placeholder="onboarding" />
              <Input label="Events" value={group.events} onChange={(value) => update(index, { events: value })} placeholder="start_game, finish_tutorial" />
              <button type="button" onClick={() => remove(index)} className="link-button justify-self-start">Remove</button>
            </div>
          </div>
        ))}
      </div>
    </Panel>
  );
}

function QueryFieldsBuilder({
  fields,
  onChange,
}: {
  fields: QueryField[];
  onChange: (fields: QueryField[]) => void;
}) {
  const update = (index: number, patch: Partial<QueryField>) => {
    onChange(fields.map((field, current) => current === index ? { ...field, ...patch } : field));
  };
  const remove = (index: number) => onChange(fields.filter((_, current) => current !== index));
  return (
    <Panel>
      <div className="mb-3 flex items-center gap-2">
        <h3 className="font-semibold text-on-surface">Query fields</h3>
        <button type="button" onClick={() => onChange([...fields, emptyQueryField()])} className="ml-auto btn-secondary">Add</button>
      </div>
      <div className="space-y-3">
        {fields.map((field, index) => (
          <div key={index} className="rounded-md border border-outline-ghost bg-surface-container p-3">
            <div className="grid gap-2 md:grid-cols-2">
              <Input label="Key" value={field.key} onChange={(value) => update(index, { key: value.trim() })} placeholder="ship.id" />
              <Input label="Label" value={field.label} onChange={(value) => update(index, { label: value })} placeholder="Ship" />
              <Input label="Source" value={field.source} onChange={(value) => update(index, { source: value.trim() })} placeholder="dimensions.ship.id" />
              <Select label="Type" value={field.type} options={["string", "number", "bool"]} onChange={(value) => update(index, { type: value as QueryField["type"] })} />
              <Input label="Aggregations" value={field.aggregations.join(", ")} onChange={(value) => update(index, { aggregations: splitLabels(value) })} placeholder="count" />
              <div className="flex items-end">
                <Checkbox label="Filterable" checked={field.filterable} onChange={(value) => update(index, { filterable: value })} />
              </div>
            </div>
            <button type="button" onClick={() => remove(index)} className="mt-2 link-button">Remove</button>
          </div>
        ))}
      </div>
    </Panel>
  );
}

function FunnelsBuilder({
  funnels,
  queryFields,
  onChange,
}: {
  funnels: FunnelDefinition[];
  queryFields: QueryField[];
  onChange: (funnels: FunnelDefinition[]) => void;
}) {
  const update = (index: number, patch: Partial<FunnelDefinition>) => {
    onChange(funnels.map((funnel, current) => current === index ? { ...funnel, ...patch } : funnel));
  };
  const updateStep = (funnelIndex: number, stepIndex: number, patch: Partial<FunnelDefinition["steps"][number]>) => {
    const funnel = funnels[funnelIndex];
    update(funnelIndex, {
      steps: funnel.steps.map((step, current) => current === stepIndex ? { ...step, ...patch } : step),
    });
  };
  const updateMatcher = (funnelIndex: number, stepIndex: number, patch: Partial<FunnelDefinition["steps"][number]["match"]>) => {
    const step = funnels[funnelIndex].steps[stepIndex];
    updateStep(funnelIndex, stepIndex, { match: { ...step.match, ...patch } });
  };
  const remove = (index: number) => onChange(funnels.filter((_, current) => current !== index));
  return (
    <Panel>
      <div className="mb-3 flex items-center gap-2">
        <h3 className="font-semibold text-on-surface">Funnels</h3>
        <button type="button" onClick={() => onChange([...funnels, emptyFunnel()])} className="ml-auto btn-secondary">Add</button>
      </div>
      <div className="space-y-3">
        {funnels.length === 0 ? <p className="text-sm text-on-surface-variant">No funnels configured.</p> : null}
        {funnels.map((funnel, funnelIndex) => (
          <div key={funnelIndex} className="rounded-md border border-outline-ghost bg-surface-container p-3">
            <div className="grid gap-2 md:grid-cols-2">
              <Input label="ID" value={funnel.id} onChange={(value) => update(funnelIndex, { id: slugConfigKey(value) })} placeholder="first_trade" />
              <Input label="Name" value={funnel.name} onChange={(value) => update(funnelIndex, { name: value })} placeholder="First trade" />
              <Input label="Description" value={funnel.description ?? ""} onChange={(value) => update(funnelIndex, { description: value })} placeholder="buy -> sell" />
              <Select label="Mode" value={funnel.mode ?? "ordered"} options={["ordered", "unordered_presence"]} onChange={(value) => update(funnelIndex, { mode: value as FunnelDefinition["mode"] })} />
              <Checkbox label="Enabled" checked={funnel.enabled !== false} onChange={(value) => update(funnelIndex, { enabled: value })} />
            </div>

            <div className="mt-3 space-y-2">
              <div className="flex items-center gap-2">
                <p className="font-medium text-on-surface">Steps</p>
                <button
                  type="button"
                  onClick={() => update(funnelIndex, { steps: [...funnel.steps, emptyFunnelStep(funnel.steps.length)] })}
                  className="ml-auto btn-secondary"
                >
                  Add Step
                </button>
              </div>
              {funnel.steps.map((step, stepIndex) => (
                <div key={stepIndex} className="rounded-md border border-outline-ghost bg-surface-container-high p-3">
                  <div className="grid gap-2 md:grid-cols-2">
                    <Input label="Step ID" value={step.id} onChange={(value) => updateStep(funnelIndex, stepIndex, { id: slugConfigKey(value) })} placeholder="started" />
                    <Input label="Label" value={step.label} onChange={(value) => updateStep(funnelIndex, stepIndex, { label: value })} placeholder="Started" />
                    <Input label="Event types" value={eventMatcherTypes(step.match)} onChange={(value) => updateMatcher(funnelIndex, stepIndex, eventTypesPatch(value))} placeholder="dock, undock" />
                    <Input label="Region" value={step.match.region_id ?? ""} onChange={(value) => updateMatcher(funnelIndex, stepIndex, { region_id: value.trim() || undefined })} placeholder="lave" />
                    <Input label="Zone" value={step.match.zone_id ?? ""} onChange={(value) => updateMatcher(funnelIndex, stepIndex, { zone_id: value.trim() || undefined })} placeholder="lave_primary" />
                    <Select label="Field" value={step.match.field_key ?? ""} options={["", ...queryFields.filter((field) => field.filterable).map((field) => field.key)]} onChange={(value) => updateMatcher(funnelIndex, stepIndex, { field_key: value || undefined, field_value: undefined })} />
                    <Input label="Field value" value={step.match.field_value === undefined ? "" : String(step.match.field_value)} onChange={(value) => updateMatcher(funnelIndex, stepIndex, { field_value: coerceFieldValue(value, queryFields.find((field) => field.key === step.match.field_key)?.type) })} placeholder="optional" />
                    {funnel.mode === "ordered" && stepIndex > 0 ? (
                      <Select label="After" value={step.after ?? funnel.steps[stepIndex - 1]?.id ?? ""} options={funnel.steps.slice(0, stepIndex).map((prior) => prior.id).filter(Boolean)} onChange={(value) => updateStep(funnelIndex, stepIndex, { after: value || undefined })} />
                    ) : null}
                    {funnel.mode === "ordered" && stepIndex > 0 ? (
                      <Input label="Within seconds" value={step.within_seconds === undefined ? "" : String(step.within_seconds)} onChange={(value) => updateStep(funnelIndex, stepIndex, { within_seconds: value.trim() ? parsePositiveInt(value, 0) : undefined })} placeholder="optional" />
                    ) : null}
                  </div>
                  <button
                    type="button"
                    onClick={() => update(funnelIndex, { steps: funnel.steps.filter((_, current) => current !== stepIndex) })}
                    className="mt-2 link-button"
                  >
                    Remove Step
                  </button>
                </div>
              ))}
            </div>
            <button type="button" onClick={() => remove(funnelIndex)} className="mt-3 link-button">Remove Funnel</button>
          </div>
        ))}
      </div>
    </Panel>
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

function eventGroupsToDrafts(groups: Record<string, string[]>): EventGroupDraft[] {
  return Object.entries(groups).map(([name, events]) => ({ name, events: events.join(", ") }));
}

function eventGroupsFromDrafts(groups: EventGroupDraft[]): Record<string, string[]> {
  return groups.reduce<Record<string, string[]>>((out, group) => {
    const name = group.name.trim();
    const events = splitLabels(group.events);
    if (name && events.length > 0) {
      out[name] = events;
    }
    return out;
  }, {});
}

function validateEventGroupDrafts(groups: EventGroupDraft[]): string {
  const seen = new Set<string>();
  for (const group of groups) {
    const name = group.name.trim();
    const events = splitLabels(group.events);
    if (!name && events.length === 0) {
      return "Remove empty event group rows before saving.";
    }
    if (!name || events.length === 0) {
      return "Event groups need both a group name and at least one event.";
    }
    if (seen.has(name)) {
      return `Duplicate event group: ${name}`;
    }
    seen.add(name);
  }
  return "";
}

function normalizeQueryField(field: QueryField): QueryField {
  return {
    key: field.key.trim(),
    source: field.source.trim(),
    type: field.type,
    label: field.label.trim(),
    filterable: field.filterable,
    aggregations: field.aggregations.map((value) => value.trim()).filter(Boolean),
  };
}

function normalizeFunnel(funnel: FunnelDefinition): FunnelDefinition {
  return {
    id: funnel.id.trim(),
    name: funnel.name.trim(),
    description: funnel.description?.trim() || undefined,
    entity: "player",
    mode: funnel.mode ?? "ordered",
    enabled: funnel.enabled !== false,
    steps: funnel.steps.map((step, index) => ({
      id: step.id.trim(),
      label: step.label.trim(),
      match: normalizeMatcher(step.match),
      after: (funnel.mode ?? "ordered") === "ordered" && index > 0 ? step.after?.trim() || funnel.steps[index - 1]?.id : undefined,
      within_seconds: (funnel.mode ?? "ordered") === "ordered" && index > 0 ? step.within_seconds : undefined,
    })),
  };
}

function normalizeMatcher(match: FunnelDefinition["steps"][number]["match"]) {
  return {
    event_type: match.event_type?.trim() || undefined,
    event_types: match.event_types?.map((value) => value.trim()).filter(Boolean),
    field_key: match.field_key?.trim() || undefined,
    field_value: match.field_value,
    region_id: match.region_id?.trim() || undefined,
    zone_id: match.zone_id?.trim() || undefined,
  };
}

function validateSchemaDrafts(queryFields: QueryField[], funnels: FunnelDefinition[]): string {
  const seenFields = new Set<string>();
  const queryFieldByKey = new Map<string, QueryField>();
  for (const field of queryFields) {
    if (!field.key || !field.source || !field.label) {
      return "Query fields need a key, source, and label.";
    }
    if (seenFields.has(field.key)) {
      return `Duplicate query field: ${field.key}`;
    }
    seenFields.add(field.key);
    queryFieldByKey.set(field.key, field);
  }
  const seenFunnels = new Set<string>();
  for (const funnel of funnels) {
    if (!funnel.id || !funnel.name) {
      return "Funnels need an ID and name.";
    }
    if (seenFunnels.has(funnel.id)) {
      return `Duplicate funnel: ${funnel.id}`;
    }
    seenFunnels.add(funnel.id);
    if (funnel.steps.length === 0) {
      return `Funnel ${funnel.id} needs at least one step.`;
    }
    const seenSteps = new Set<string>();
    for (const step of funnel.steps) {
      if (!step.id || !step.label) {
        return `Funnel ${funnel.id} has a step missing an ID or label.`;
      }
      if (seenSteps.has(step.id)) {
        return `Funnel ${funnel.id} has a duplicate step: ${step.id}`;
      }
      seenSteps.add(step.id);
      const hasMatcher = !!(step.match.event_type || step.match.event_types?.length || step.match.field_key || step.match.region_id || step.match.zone_id);
      if (!hasMatcher) {
        return `Step ${step.id} needs at least one matcher.`;
      }
      if (step.match.field_key && step.match.field_value !== undefined) {
        const field = queryFieldByKey.get(step.match.field_key);
        if (!field) {
          return `Step ${step.id} uses an unknown field.`;
        }
        if (field.type === "number" && typeof step.match.field_value !== "number") {
          return `Step ${step.id} field value must be a number.`;
        }
        if (field.type === "bool" && typeof step.match.field_value !== "boolean") {
          return `Step ${step.id} field value must be true or false.`;
        }
      }
    }
  }
  return "";
}

function emptyQueryField(): QueryField {
  return {
    key: "",
    source: "",
    type: "string",
    label: "",
    filterable: true,
    aggregations: ["count"],
  };
}

function emptyFunnel(): FunnelDefinition {
  return {
    id: "",
    name: "",
    description: "",
    entity: "player",
    mode: "ordered",
    enabled: true,
    steps: [emptyFunnelStep(0)],
  };
}

function emptyFunnelStep(index: number): FunnelDefinition["steps"][number] {
  return {
    id: index === 0 ? "started" : `step_${index + 1}`,
    label: index === 0 ? "Started" : `Step ${index + 1}`,
    match: {},
  };
}

function eventMatcherTypes(match: FunnelDefinition["steps"][number]["match"]): string {
  if (match.event_types?.length) {
    return match.event_types.join(", ");
  }
  return match.event_type ?? "";
}

function eventTypesPatch(value: string): Partial<FunnelDefinition["steps"][number]["match"]> {
  const eventTypes = splitLabels(value);
  if (eventTypes.length === 0) {
    return { event_type: undefined, event_types: undefined };
  }
  if (eventTypes.length === 1) {
    return { event_type: eventTypes[0], event_types: undefined };
  }
  return { event_type: undefined, event_types: eventTypes };
}

function coerceFieldValue(value: string, type?: QueryField["type"]): string | number | boolean | undefined {
  const trimmed = value.trim();
  if (!trimmed) return undefined;
  if (type === "number") {
    const parsed = Number.parseFloat(trimmed);
    return Number.isFinite(parsed) ? parsed : trimmed;
  }
  if (type === "bool") {
    if (trimmed === "true") return true;
    if (trimmed === "false") return false;
  }
  return trimmed;
}

function slugConfigKey(value: string) {
  return value.trim().toLowerCase().replace(/[^a-z0-9_.-]/g, "_");
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

const defaultFunnels: FunnelDefinition[] = [];
