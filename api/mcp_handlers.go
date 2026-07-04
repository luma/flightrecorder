package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"github.com/luma/flightrecorder/services"
)

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func makeMCPJSONRPC(adminSvc services.Admin) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		if !mcpAcceptsJSON(c) {
			c.JSON(consts.StatusNotAcceptable, map[string]string{"error": "MCP requests must accept application/json"})
			return
		}
		var req mcpRequest
		if err := json.Unmarshal(c.Request.Body(), &req); err != nil {
			c.JSON(consts.StatusBadRequest, mcpResponse{JSONRPC: "2.0", Error: &mcpError{Code: -32700, Message: "parse error"}})
			return
		}
		if len(req.ID) == 0 {
			c.SetStatusCode(consts.StatusAccepted)
			return
		}
		result, rpcErr := dispatchMCP(ctx, adminSvc, req)
		resp := mcpResponse{JSONRPC: "2.0", ID: req.ID, Result: result, Error: rpcErr}
		c.JSON(consts.StatusOK, resp)
	}
}

func mcpAcceptsJSON(c *app.RequestContext) bool {
	accept := strings.TrimSpace(string(c.GetHeader("Accept")))
	if accept == "" || accept == "*/*" {
		return true
	}
	for _, item := range strings.Split(accept, ",") {
		item = strings.TrimSpace(strings.SplitN(item, ";", 2)[0])
		if item == "application/json" || item == "application/*" || item == "*/*" {
			return true
		}
	}
	return false
}

func dispatchMCP(ctx context.Context, adminSvc services.Admin, req mcpRequest) (any, *mcpError) {
	switch req.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities": map[string]any{
				"tools":     map[string]any{},
				"resources": map[string]any{},
			},
			"serverInfo": map[string]any{"name": "flightrecorder", "version": "dev"},
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": mcpTools()}, nil
	case "tools/call":
		var params mcpToolCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &mcpError{Code: -32602, Message: "invalid tool call params"}
		}
		return callMCPTool(ctx, adminSvc, params)
	case "resources/list":
		return map[string]any{"resources": []any{}}, nil
	case "resources/templates/list":
		return map[string]any{"resourceTemplates": []any{}}, nil
	default:
		return nil, &mcpError{Code: -32601, Message: "method not found"}
	}
}

func callMCPTool(ctx context.Context, adminSvc services.Admin, params mcpToolCallParams) (any, *mcpError) {
	session, err := services.AgentSessionFromContext(ctx)
	if !err {
		return nil, &mcpError{Code: -32001, Message: "agent session missing"}
	}
	switch params.Name {
	case "projects.list":
		if rpcErr := requireMCPScope(session, services.MCPReadScope); rpcErr != nil {
			return nil, rpcErr
		}
		projects, err := adminSvc.ListProjects(ctx)
		if err != nil {
			return nil, serviceRPCError(err)
		}
		if !session.AllProjects {
			filtered := make([]services.ProjectSummary, 0, len(projects))
			for _, project := range projects {
				if session.CanAccessProject(project.ProjectID) {
					filtered = append(filtered, project)
				}
			}
			projects = filtered
		}
		return mcpStructured(projects), nil
	case "projects.get_settings":
		if rpcErr := requireMCPScope(session, services.MCPReadScope); rpcErr != nil {
			return nil, rpcErr
		}
		var args struct {
			ProjectKey string `json:"project_key"`
		}
		if err := decodeMCPArgs(params.Arguments, &args); err != nil {
			return nil, invalidMCPArgs(err)
		}
		if rpcErr := requireMCPProject(session, args.ProjectKey); rpcErr != nil {
			return nil, rpcErr
		}
		settings, err := adminSvc.Settings(ctx, args.ProjectKey)
		if err != nil {
			return nil, serviceRPCError(err)
		}
		return mcpStructured(settings), nil
	case "projects.create":
		if rpcErr := requireMCPScope(session, services.MCPWriteScope); rpcErr != nil {
			return nil, rpcErr
		}
		if !session.AllProjects {
			return nil, &mcpError{Code: -32003, Message: "all-projects access is required to create projects"}
		}
		var req services.CreateProjectRequest
		if err := decodeMCPArgs(params.Arguments, &req); err != nil {
			return nil, invalidMCPArgs(err)
		}
		project, err := adminSvc.CreateProject(ctx, req)
		if err != nil {
			return nil, serviceRPCError(err)
		}
		return mcpStructured(project), nil
	case "projects.update":
		if rpcErr := requireMCPScope(session, services.MCPWriteScope); rpcErr != nil {
			return nil, rpcErr
		}
		var req services.CreateProjectRequest
		if err := decodeMCPArgs(params.Arguments, &req); err != nil {
			return nil, invalidMCPArgs(err)
		}
		if rpcErr := requireMCPProject(session, req.ProjectID); rpcErr != nil {
			return nil, rpcErr
		}
		project, err := adminSvc.UpdateProject(ctx, req)
		if err != nil {
			return nil, serviceRPCError(err)
		}
		return mcpStructured(project), nil
	case "metrics.summary":
		filter, rpcErr := mcpTimeFilter(session, params.Arguments)
		if rpcErr != nil {
			return nil, rpcErr
		}
		summary, err := adminSvc.Summary(ctx, filter)
		if err != nil {
			return nil, serviceRPCError(err)
		}
		return mcpStructured(summary), nil
	case "funnels.query":
		filter, rpcErr := mcpTimeFilter(session, params.Arguments)
		if rpcErr != nil {
			return nil, rpcErr
		}
		funnels, err := adminSvc.Funnels(ctx, filter)
		if err != nil {
			return nil, serviceRPCError(err)
		}
		return mcpStructured(funnels), nil
	case "reports.list":
		if rpcErr := requireMCPScope(session, services.MCPReadScope); rpcErr != nil {
			return nil, rpcErr
		}
		var args struct {
			ProjectKey string `json:"project_key"`
			Status     string `json:"status,omitempty"`
			Label      string `json:"label,omitempty"`
			Limit      int32  `json:"limit,omitempty"`
			Offset     int32  `json:"offset,omitempty"`
		}
		if err := decodeMCPArgs(params.Arguments, &args); err != nil {
			return nil, invalidMCPArgs(err)
		}
		if rpcErr := requireMCPProject(session, args.ProjectKey); rpcErr != nil {
			return nil, rpcErr
		}
		reports, err := adminSvc.ListReports(ctx, services.ReportListFilter{
			ProjectID: args.ProjectKey,
			Status:    services.OptionalString(args.Status),
			Label:     services.OptionalString(args.Label),
			Limit:     args.Limit,
			Offset:    args.Offset,
		})
		if err != nil {
			return nil, serviceRPCError(err)
		}
		return mcpStructured(reports), nil
	case "reports.get":
		if rpcErr := requireMCPScope(session, services.MCPReadScope); rpcErr != nil {
			return nil, rpcErr
		}
		var args struct {
			ProjectKey string `json:"project_key"`
			ReportID   string `json:"report_id"`
		}
		if err := decodeMCPArgs(params.Arguments, &args); err != nil {
			return nil, invalidMCPArgs(err)
		}
		if rpcErr := requireMCPProject(session, args.ProjectKey); rpcErr != nil {
			return nil, rpcErr
		}
		report, err := adminSvc.GetReport(ctx, args.ProjectKey, args.ReportID)
		if err != nil {
			return nil, serviceRPCError(err)
		}
		return mcpStructured(report), nil
	case "events.list":
		if rpcErr := requireMCPScope(session, services.MCPReadScope); rpcErr != nil {
			return nil, rpcErr
		}
		var args struct {
			ProjectKey   string `json:"project_key"`
			From         string `json:"from,omitempty"`
			To           string `json:"to,omitempty"`
			EventType    string `json:"event_type,omitempty"`
			RegionID     string `json:"region_id,omitempty"`
			ZoneID       string `json:"zone_id,omitempty"`
			PlayerID     string `json:"player_id,omitempty"`
			GameVersion  string `json:"game_version,omitempty"`
			BuildChannel string `json:"build_channel,omitempty"`
			FieldKey     string `json:"field_key,omitempty"`
			FieldValue   string `json:"field_value,omitempty"`
			Limit        int32  `json:"limit,omitempty"`
			Offset       int32  `json:"offset,omitempty"`
		}
		if err := decodeMCPArgs(params.Arguments, &args); err != nil {
			return nil, invalidMCPArgs(err)
		}
		if rpcErr := requireMCPProject(session, args.ProjectKey); rpcErr != nil {
			return nil, rpcErr
		}
		filter, err := services.ParseTimeRange(args.ProjectKey, args.From, args.To)
		if err != nil {
			return nil, serviceRPCError(err)
		}
		events, err := adminSvc.ListEvents(ctx, services.EventListFilter{
			TimeProjectFilter: filter,
			EventType:         services.OptionalString(args.EventType),
			RegionID:          services.OptionalString(args.RegionID),
			ZoneID:            services.OptionalString(args.ZoneID),
			PlayerID:          services.OptionalString(args.PlayerID),
			GameVersion:       services.OptionalString(args.GameVersion),
			BuildChannel:      services.OptionalString(args.BuildChannel),
			FieldKey:          services.OptionalString(args.FieldKey),
			FieldValue:        services.OptionalString(args.FieldValue),
			Limit:             args.Limit,
			Offset:            args.Offset,
		})
		if err != nil {
			return nil, serviceRPCError(err)
		}
		return mcpStructured(events), nil
	case "events.types":
		if rpcErr := requireMCPScope(session, services.MCPReadScope); rpcErr != nil {
			return nil, rpcErr
		}
		var args struct {
			ProjectKey string `json:"project_key"`
		}
		if err := decodeMCPArgs(params.Arguments, &args); err != nil {
			return nil, invalidMCPArgs(err)
		}
		if rpcErr := requireMCPProject(session, args.ProjectKey); rpcErr != nil {
			return nil, rpcErr
		}
		types, err := adminSvc.EventTypes(ctx, args.ProjectKey)
		if err != nil {
			return nil, serviceRPCError(err)
		}
		return mcpStructured(types), nil
	default:
		return nil, &mcpError{Code: -32602, Message: "unknown tool"}
	}
}

func mcpTools() []map[string]any {
	return []map[string]any{
		mcpTool("projects.list", "List projects available to this agent.", map[string]any{"type": "object", "properties": map[string]any{}}),
		mcpTool("projects.get_settings", "Get a project's schema, funnels, report config, and settings.", projectKeySchema()),
		mcpTool("projects.create", "Create a project. Requires all-projects access.", createProjectSchema()),
		mcpTool("projects.update", "Update an existing project. Cannot create missing projects.", createProjectSchema()),
		mcpTool("metrics.summary", "Query summary metrics for a project and time range.", timeProjectSchema()),
		mcpTool("funnels.query", "Query configured funnels for a project and time range.", timeProjectSchema()),
		mcpTool("reports.list", "List feedback reports for a project.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_key": map[string]any{"type": "string"},
				"status":      map[string]any{"type": "string"},
				"label":       map[string]any{"type": "string"},
				"limit":       map[string]any{"type": "integer"},
				"offset":      map[string]any{"type": "integer"},
			},
			"required": []string{"project_key"},
		}),
		mcpTool("reports.get", "Get a feedback report with trace and notes.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_key": map[string]any{"type": "string"},
				"report_id":   map[string]any{"type": "string"},
			},
			"required": []string{"project_key", "report_id"},
		}),
		mcpTool("events.list", "List telemetry events for a project.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_key":   map[string]any{"type": "string"},
				"from":          map[string]any{"type": "string", "format": "date-time"},
				"to":            map[string]any{"type": "string", "format": "date-time"},
				"event_type":    map[string]any{"type": "string"},
				"region_id":     map[string]any{"type": "string"},
				"zone_id":       map[string]any{"type": "string"},
				"player_id":     map[string]any{"type": "string"},
				"game_version":  map[string]any{"type": "string"},
				"build_channel": map[string]any{"type": "string"},
				"field_key":     map[string]any{"type": "string"},
				"field_value":   map[string]any{"type": "string"},
				"limit":         map[string]any{"type": "integer"},
				"offset":        map[string]any{"type": "integer"},
			},
			"required": []string{"project_key"},
		}),
		mcpTool("events.types", "List event types seen for a project.", projectKeySchema()),
	}
}

func mcpTool(name string, description string, inputSchema map[string]any) map[string]any {
	return map[string]any{
		"name":        name,
		"description": description,
		"inputSchema": inputSchema,
	}
}

func projectKeySchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{"project_key": map[string]any{"type": "string"}},
		"required":   []string{"project_key"},
	}
}

func timeProjectSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_key": map[string]any{"type": "string"},
			"from":        map[string]any{"type": "string", "format": "date-time"},
			"to":          map[string]any{"type": "string", "format": "date-time"},
		},
		"required": []string{"project_key"},
	}
}

func createProjectSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":      map[string]any{"type": "string"},
			"display_name":    map[string]any{"type": "string"},
			"validation_mode": map[string]any{"type": "string"},
		},
		"required": []string{"project_id", "display_name"},
	}
}

func decodeMCPArgs(raw json.RawMessage, out any) error {
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	return json.Unmarshal(raw, out)
}

func mcpTimeFilter(session services.AgentSession, raw json.RawMessage) (services.TimeProjectFilter, *mcpError) {
	if rpcErr := requireMCPScope(session, services.MCPReadScope); rpcErr != nil {
		return services.TimeProjectFilter{}, rpcErr
	}
	var args struct {
		ProjectKey string `json:"project_key"`
		From       string `json:"from,omitempty"`
		To         string `json:"to,omitempty"`
	}
	if err := decodeMCPArgs(raw, &args); err != nil {
		return services.TimeProjectFilter{}, invalidMCPArgs(err)
	}
	if rpcErr := requireMCPProject(session, args.ProjectKey); rpcErr != nil {
		return services.TimeProjectFilter{}, rpcErr
	}
	filter, err := services.ParseTimeRange(args.ProjectKey, args.From, args.To)
	if err != nil {
		return services.TimeProjectFilter{}, serviceRPCError(err)
	}
	return filter, nil
}

func requireMCPScope(session services.AgentSession, scope string) *mcpError {
	if !session.HasScope(scope) {
		return &mcpError{Code: -32003, Message: "agent is missing required scope " + scope}
	}
	return nil
}

func requireMCPProject(session services.AgentSession, projectKey string) *mcpError {
	projectKey = strings.TrimSpace(projectKey)
	if projectKey == "" {
		return &mcpError{Code: -32602, Message: "project_key is required"}
	}
	if !session.CanAccessProject(projectKey) {
		return &mcpError{Code: -32003, Message: "agent is not allowed to access this project"}
	}
	return nil
}

func mcpStructured(value any) map[string]any {
	text, _ := json.MarshalIndent(value, "", "  ")
	return map[string]any{
		"content": []map[string]string{
			{"type": "text", "text": string(text)},
		},
		"structuredContent": value,
	}
}

func invalidMCPArgs(err error) *mcpError {
	return &mcpError{Code: -32602, Message: "invalid arguments: " + err.Error()}
}

func serviceRPCError(err error) *mcpError {
	return &mcpError{Code: -32000, Message: fmt.Sprintf("%v", err)}
}
