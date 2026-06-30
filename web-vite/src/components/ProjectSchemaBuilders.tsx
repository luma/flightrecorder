import type { ReactNode } from "react";
import type { FunnelDefinition, QueryField } from "../api";

export type EventGroupDraft = {
  name: string;
  events: string;
};

export function EventGroupsBuilder({
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

export function QueryFieldsBuilder({
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

export function FunnelsBuilder({
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

export function eventGroupsToDrafts(groups: Record<string, string[]>): EventGroupDraft[] {
  return Object.entries(groups).map(([name, events]) => ({ name, events: events.join(", ") }));
}

export function eventGroupsFromDrafts(groups: EventGroupDraft[]): Record<string, string[]> {
  return groups.reduce<Record<string, string[]>>((out, group) => {
    const name = group.name.trim();
    const events = splitLabels(group.events);
    if (name && events.length > 0) {
      out[name] = events;
    }
    return out;
  }, {});
}

export function validateEventGroupDrafts(groups: EventGroupDraft[]): string {
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

export function normalizeQueryField(field: QueryField): QueryField {
  return {
    key: field.key.trim(),
    source: field.source.trim(),
    type: field.type,
    label: field.label.trim(),
    filterable: field.filterable,
    aggregations: field.aggregations.map((value) => value.trim()).filter(Boolean),
  };
}

export function normalizeFunnel(funnel: FunnelDefinition): FunnelDefinition {
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

export function validateSchemaDrafts(queryFields: QueryField[], funnels: FunnelDefinition[]): string {
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
      if (step.match.field_key) {
        const field = queryFieldByKey.get(step.match.field_key);
        if (!field) {
          return `Step ${step.id} uses an unknown field.`;
        }
        if (!field.filterable) {
          return `Step ${step.id} field must be filterable.`;
        }
        if (step.match.field_value === undefined) {
          continue;
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

function Panel({ children }: { children: ReactNode }) {
  return <section className="rounded-md border border-outline-ghost bg-surface-container-low p-4">{children}</section>;
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

function parsePositiveInt(value: string, fallback: number) {
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
}

function splitLabels(value: string) {
  return value.split(",").map((label) => label.trim()).filter(Boolean);
}

function slugConfigKey(value: string) {
  return value.trim().toLowerCase().replace(/[^a-z0-9_.-]/g, "_");
}
