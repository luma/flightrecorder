const ADMIN_API_BASE = "/api/admin/v1";

const userTimezone = Intl.DateTimeFormat().resolvedOptions().timeZone;

export interface User {
  email: string;
}

async function apiFetch<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${ADMIN_API_BASE}${path}`, {
    ...options,
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "X-Timezone": userTimezone,
      ...options?.headers,
    },
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(body.error || res.statusText);
  }
  if (res.status === 204 || res.headers.get("content-length") === "0") {
    return undefined as T;
  }
  return res.json();
}

function queryString(params: object): string {
  const query = new URLSearchParams();
  Object.entries(params as Record<string, string | number | undefined | null>).forEach(([key, value]) => {
    if (value === undefined || value === null || value === "") return;
    query.set(key, String(value));
  });
  const encoded = query.toString();
  return encoded ? `?${encoded}` : "";
}

export interface Summary {
  event_count: number;
  player_count: number;
  session_count: number;
  death_count: number;
  report_count: number;
  opt_in_count: number;
  from: string;
  to: string;
}

export interface EventSummary {
  id: string;
  player_id: string;
  event_type: string;
  real_ts: string;
  game_time: number;
  region_id: string;
  zone_id: string;
  coordinates: number[];
  game_version: string;
  build_channel: string;
  commit_sha: string;
  platform: string;
  context: Record<string, unknown>;
  metrics: Record<string, unknown>;
  dimensions: Record<string, unknown>;
  payload: Record<string, unknown>;
  event_json: Record<string, unknown>;
  fields: Record<string, unknown>;
}

export interface TraceEvent {
  id: string;
  event_type: string;
  real_ts: string;
  game_time: number;
  region_id: string;
  zone_id: string;
  coordinates: number[];
  context: Record<string, unknown>;
  metrics: Record<string, unknown>;
  dimensions: Record<string, unknown>;
  fields: Record<string, unknown>;
  payload: Record<string, unknown>;
}

export interface ReportSummary {
  report_id: string;
  status: string;
  labels: string[];
  mood: number;
  mood_label: string;
  notes_preview: string;
  screenshot_object_key?: string;
  created_at: string;
  player_id: string;
  region_id: string;
  zone_id: string;
  context?: Record<string, unknown>;
  metrics?: Record<string, unknown>;
  dimensions?: Record<string, unknown>;
  payload?: Record<string, unknown>;
}

export interface ReportNote {
  id: string;
  note: string;
  created_at: string;
}

export interface ReportDetail extends ReportSummary {
  screenshot_storage_error?: string;
  coordinates: number[];
  trace: TraceEvent[];
  notes: ReportNote[];
}

export interface HeatmapCell {
  region_id: string;
  zone_id?: string;
  grid_x?: number;
  grid_z?: number;
  event_type: string;
  event_count: number;
}

export interface EventTypeSummary {
  event_type: string;
  event_count: number;
  last_seen_at: string;
  sample_payload: Record<string, unknown>;
}

export interface QueryField {
  key: string;
  source: string;
  type: "string" | "number" | "bool";
  label: string;
  filterable: boolean;
  aggregations: string[];
}

export interface EventMatcher {
  event_type?: string;
  event_types?: string[];
  field_key?: string;
  field_value?: string | number | boolean;
  region_id?: string;
  zone_id?: string;
}

export interface FunnelStep {
  id: string;
  label: string;
  match: EventMatcher;
  after?: string;
  within_seconds?: number;
}

export interface FunnelDefinition {
  id: string;
  name: string;
  description?: string;
  entity: "player";
  mode?: "ordered" | "unordered_presence";
  enabled?: boolean;
  steps: FunnelStep[];
}

export interface FunnelStepSummary {
  id: string;
  label: string;
  count: number;
  rate: number;
}

export interface FunnelSummary {
  id: string;
  name: string;
  description: string;
  entity: string;
  started: number;
  completed: number;
  rate: number;
  dropoff: string;
  steps: FunnelStepSummary[];
}

export interface AdminFilters {
  project_id: string;
  from?: string;
  to?: string;
  event_type?: string;
  region_id?: string;
  zone_id?: string;
  player_id?: string;
  game_version?: string;
  build_channel?: string;
  field_key?: string;
  field_value?: string;
  status?: string;
  label?: string;
  limit?: number;
}

export interface UpdateReportRequest {
  status: string;
  labels: string[];
  note?: string;
}

export interface IngestTokenSummary {
  id: string;
  name: string;
  enabled: boolean;
  expires_at?: string;
  last_used_at?: string;
  created_at: string;
}

export interface SettingsResponse {
  project: {
    project_id: string;
    display_name: string;
    validation_mode: string;
    ingest_config: Record<string, unknown>;
    retention_config: Record<string, unknown>;
    map_config: Record<string, unknown>;
    report_config: Record<string, unknown>;
    event_groups: Record<string, unknown>;
    query_fields: QueryField[];
    funnels: FunnelDefinition[];
  };
  tokens: IngestTokenSummary[];
}

export interface ProjectSummary {
  project_id: string;
  display_name: string;
  validation_mode: string;
  created_at: string;
  updated_at: string;
}

export interface CreateProjectRequest {
  project_id: string;
  display_name: string;
  validation_mode: "warn" | "strict";
  ingest_config: Record<string, unknown>;
  retention_config: Record<string, unknown>;
  map_config: Record<string, unknown>;
  report_config: Record<string, unknown>;
  event_groups: Record<string, string[]>;
  query_fields: QueryField[];
  funnels: FunnelDefinition[];
}

export interface CreateIngestTokenResponse {
  token: string;
  summary: IngestTokenSummary;
}

export const api = {
  me: () => apiFetch<{ user: User }>("/auth/me").then((resp) => resp.user),
  devLogin: (email: string) =>
    apiFetch<{ user: User }>("/auth/dev-login", {
      method: "POST",
      body: JSON.stringify({ email }),
    }),
  logout: () => apiFetch<void>("/auth/logout", { method: "POST" }),
  summary: (filters: AdminFilters) => apiFetch<Summary>(`/summary${queryString(filters)}`),
  events: (filters: AdminFilters) =>
    apiFetch<{ events: EventSummary[] }>(`/events${queryString({ ...filters, limit: filters.limit ?? 100 })}`),
  playerTrace: (projectID: string, playerID: string) =>
    apiFetch<{ events: TraceEvent[] }>(
      `/players/${encodeURIComponent(playerID)}/trace?project_id=${encodeURIComponent(projectID)}&limit=100`,
    ),
  regionHeatmap: (filters: AdminFilters) =>
    apiFetch<{ cells: HeatmapCell[] }>(`/heatmap/regions${queryString(filters)}`),
  zoneHeatmap: (filters: AdminFilters) =>
    apiFetch<{ cells: HeatmapCell[] }>(`/heatmap/zones${queryString(filters)}`),
  funnels: (filters: AdminFilters) => apiFetch<{ funnels: FunnelSummary[] }>(`/funnels${queryString(filters)}`),
  reports: (filters: AdminFilters) =>
    apiFetch<{ reports: ReportSummary[] }>(`/reports${queryString({ ...filters, limit: filters.limit ?? 100 })}`),
  reportDetail: (projectID: string, reportID: string) =>
    apiFetch<ReportDetail>(`/reports/${encodeURIComponent(reportID)}${queryString({ project_id: projectID })}`),
  reportScreenshotURL: (projectID: string, reportID: string) =>
    `${ADMIN_API_BASE}/reports/${encodeURIComponent(reportID)}/screenshot${queryString({ project_id: projectID })}`,
  updateReport: (projectID: string, reportID: string, body: UpdateReportRequest) =>
    apiFetch<{ report_id: string; status: string; labels: string[]; updated_at: string }>(
      `/reports/${encodeURIComponent(reportID)}${queryString({ project_id: projectID })}`,
      {
        method: "PATCH",
        body: JSON.stringify(body),
      },
    ),
  eventTypes: (projectID: string) =>
    apiFetch<{ event_types: EventTypeSummary[] }>(`/event-types?project_id=${encodeURIComponent(projectID)}`),
  projects: () => apiFetch<{ projects: ProjectSummary[] }>("/projects"),
  createProject: (body: CreateProjectRequest) =>
    apiFetch<SettingsResponse["project"]>("/projects", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  settings: (projectID: string) =>
    apiFetch<SettingsResponse>(`/settings${queryString({ project_id: projectID })}`),
  createIngestToken: (projectID: string, name: string) =>
    apiFetch<CreateIngestTokenResponse>(`/settings/ingest-tokens${queryString({ project_id: projectID })}`, {
      method: "POST",
      body: JSON.stringify({ name }),
    }),
  setIngestTokenEnabled: (projectID: string, tokenID: string, enabled: boolean) =>
    apiFetch<IngestTokenSummary>(
      `/settings/ingest-tokens/${encodeURIComponent(tokenID)}${queryString({ project_id: projectID })}`,
      {
        method: "PATCH",
        body: JSON.stringify({ enabled }),
      },
    ),
};
