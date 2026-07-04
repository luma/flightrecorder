const ADMIN_API_BASE = "/api/admin/v1";

const userTimezone = Intl.DateTimeFormat().resolvedOptions().timeZone;

export interface User {
  email: string;
  name?: string;
  picture?: string;
}

async function apiFetch<T>(path: string, options?: RequestInit): Promise<T> {
  return fetchJSON<T>(`${ADMIN_API_BASE}${path}`, options);
}

async function fetchJSON<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(path, {
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

export interface RejectedEventGroup {
  event_type: string;
  reason_code: string;
  reason_message: string;
  game_version: string;
  build_channel: string;
  event_count: number;
  first_seen_at: string;
  last_seen_at: string;
  sample_event: Record<string, unknown>;
}

export interface RejectedEventsResponse {
  groups: RejectedEventGroup[];
  active_group_count: number;
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

export interface AuthConfig {
  google_enabled: boolean;
  dev_login_enabled: boolean;
}

export interface AdminUserSummary {
  id: string;
  email: string;
  name: string;
  picture: string;
  role: string;
  enabled: boolean;
  provider: string;
  last_login_at?: string;
  created_at: string;
  updated_at: string;
}

export interface AdminInvitationSummary {
  id: string;
  email: string;
  expires_at: string;
  created_at: string;
  created_by_email?: string;
}

export interface CreateAdminInvitationResponse {
  invitation: AdminInvitationSummary;
  token: string;
}

export interface AgentAuthorizationSummary {
  id: string;
  client_id: string;
  client_name: string;
  created_by_admin_user_id?: string;
  created_by_email?: string;
  all_projects: boolean;
  project_keys: string[];
  scopes: string[];
  enabled: boolean;
  expires_at: string;
  activated_at?: string;
  last_used_at?: string;
  created_at: string;
  updated_at: string;
}

export interface MCPConsentDetails {
  client_name: string;
  projects: ProjectSummary[];
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
    event_groups: Record<string, string[]>;
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
  googleLoginURL: (returnPath?: string) => `${ADMIN_API_BASE}/auth/google/start${queryString({ return_path: returnPath })}`,
  authConfig: () => apiFetch<AuthConfig>("/auth/config"),
  me: () => apiFetch<{ user: User }>("/auth/me").then((resp) => resp.user),
  devLogin: (email: string) =>
    apiFetch<{ user: User }>("/auth/dev-login", {
      method: "POST",
      body: JSON.stringify({ email }),
    }),
  acceptInviteCode: (code: string) =>
    apiFetch<{ user: User }>("/auth/invite-code", {
      method: "POST",
      body: JSON.stringify({ code }),
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
  rejectedEvents: (projectID: string) =>
    apiFetch<RejectedEventsResponse>(`/rejected-events${queryString({ project_id: projectID })}`),
  rejectedEventCount: (projectID: string) =>
    apiFetch<{ active_group_count: number }>(`/rejected-events/count${queryString({ project_id: projectID })}`),
  acknowledgeRejectedEvents: (projectID: string) =>
    apiFetch<{ acknowledged: boolean }>(`/rejected-events/acknowledge${queryString({ project_id: projectID })}`, {
      method: "POST",
    }),
  projects: () => apiFetch<{ projects: ProjectSummary[] }>("/projects"),
  createProject: (body: CreateProjectRequest) =>
    apiFetch<SettingsResponse["project"]>("/projects", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  updateProject: (body: CreateProjectRequest) =>
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
  adminUsers: () => apiFetch<{ users: AdminUserSummary[] }>("/users"),
  setAdminUserEnabled: (userID: string, enabled: boolean) =>
    apiFetch<AdminUserSummary>(`/users/${encodeURIComponent(userID)}`, {
      method: "PATCH",
      body: JSON.stringify({ enabled }),
    }),
  adminInvitations: () => apiFetch<{ invitations: AdminInvitationSummary[] }>("/invitations"),
  createAdminInvitation: (email: string) =>
    apiFetch<CreateAdminInvitationResponse>("/invitations", {
      method: "POST",
      body: JSON.stringify({ email }),
    }),
  deleteAdminInvitation: (invitationID: string) =>
    apiFetch<AdminInvitationSummary>(`/invitations/${encodeURIComponent(invitationID)}`, {
      method: "DELETE",
    }),
  agentAuthorizations: () => apiFetch<{ authorizations: AgentAuthorizationSummary[] }>("/agent-authorizations"),
  setAgentAuthorizationEnabled: (authorizationID: string, enabled: boolean) =>
    apiFetch<AgentAuthorizationSummary>(`/agent-authorizations/${encodeURIComponent(authorizationID)}`, {
      method: "PATCH",
      body: JSON.stringify({ enabled }),
    }),
  mcpConsentDetails: (request: string) =>
    fetchJSON<MCPConsentDetails>(`/api/mcp/oauth/consent${queryString({ request })}`),
  confirmMCPConsent: (request: string, body: { all_projects: boolean; project_keys: string[] }) =>
    fetchJSON<{ redirect_uri: string }>(`/api/mcp/oauth/consent${queryString({ request })}`, {
      method: "POST",
      body: JSON.stringify(body),
    }),
};
