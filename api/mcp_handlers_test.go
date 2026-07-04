package api

import "testing"

// TestMCPProjectMutationToolsExposeFullSchema ensures projects.create and
// projects.update advertise the complete services.CreateProjectRequest config
// shape so MCP agents can supply full project configuration.
func TestMCPProjectMutationToolsExposeFullSchema(t *testing.T) {
	tools := mcpTools()

	byName := make(map[string]map[string]any, len(tools))
	for _, tool := range tools {
		name, _ := tool["name"].(string)
		byName[name] = tool
	}

	topLevelFields := []string{
		"project_id",
		"display_name",
		"validation_mode",
		"ingest_config",
		"retention_config",
		"map_config",
		"report_config",
		"event_groups",
		"query_fields",
		"funnels",
	}

	for _, name := range []string{"projects.create", "projects.update"} {
		tool, ok := byName[name]
		if !ok {
			t.Fatalf("tool %q not advertised in tools/list", name)
		}
		props := schemaProperties(t, tool, name)
		for _, field := range topLevelFields {
			if _, ok := props[field]; !ok {
				t.Errorf("tool %q schema is missing top-level field %q", name, field)
			}
		}

		// query_fields must describe its array item object.
		queryFields := mustMap(t, props["query_fields"], name+".query_fields")
		queryItems := mustMap(t, queryFields["items"], name+".query_fields.items")
		queryItemProps := mustMap(t, queryItems["properties"], name+".query_fields.items.properties")
		for _, field := range []string{"key", "source", "type", "label", "filterable", "aggregations"} {
			if _, ok := queryItemProps[field]; !ok {
				t.Errorf("tool %q query_fields item missing field %q", name, field)
			}
		}

		// funnels must describe nested steps and matcher.
		funnels := mustMap(t, props["funnels"], name+".funnels")
		funnelItems := mustMap(t, funnels["items"], name+".funnels.items")
		funnelItemProps := mustMap(t, funnelItems["properties"], name+".funnels.items.properties")
		for _, field := range []string{"id", "name", "entity", "steps"} {
			if _, ok := funnelItemProps[field]; !ok {
				t.Errorf("tool %q funnels item missing field %q", name, field)
			}
		}
		steps := mustMap(t, funnelItemProps["steps"], name+".funnels.items.steps")
		stepItems := mustMap(t, steps["items"], name+".funnels.items.steps.items")
		stepProps := mustMap(t, stepItems["properties"], name+".funnels.items.steps.items.properties")
		if _, ok := stepProps["match"]; !ok {
			t.Errorf("tool %q funnel step missing match", name)
		}
	}

	// projects.create must require the full configuration shape so agents
	// cannot create an incomplete project that silently falls back to defaults.
	createRequired := schemaRequired(t, byName["projects.create"], "projects.create")
	for _, field := range topLevelFields {
		if !createRequired[field] {
			t.Errorf("projects.create schema must require field %q", field)
		}
	}

	// projects.update targets an existing project, so it only requires identity
	// fields and may narrow the update to specific config sections.
	updateRequired := schemaRequired(t, byName["projects.update"], "projects.update")
	for _, field := range []string{"project_id", "display_name"} {
		if !updateRequired[field] {
			t.Errorf("projects.update schema must require field %q", field)
		}
	}
	for _, field := range []string{"ingest_config", "event_groups", "query_fields", "funnels"} {
		if updateRequired[field] {
			t.Errorf("projects.update schema should not require config field %q", field)
		}
	}
}

func schemaRequired(t *testing.T, tool map[string]any, name string) map[string]bool {
	t.Helper()
	schema := mustMap(t, tool["inputSchema"], name+".inputSchema")
	raw, ok := schema["required"].([]string)
	if !ok {
		t.Fatalf("expected %s.inputSchema.required to be []string, got %T", name, schema["required"])
	}
	set := make(map[string]bool, len(raw))
	for _, field := range raw {
		set[field] = true
	}
	return set
}

func schemaProperties(t *testing.T, tool map[string]any, name string) map[string]any {
	t.Helper()
	schema := mustMap(t, tool["inputSchema"], name+".inputSchema")
	return mustMap(t, schema["properties"], name+".inputSchema.properties")
}

func mustMap(t *testing.T, v any, path string) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected %s to be an object, got %T", path, v)
	}
	return m
}
