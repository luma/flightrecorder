package api

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/cloudwego/hertz/pkg/route"

	"github.com/luma/flightrecorder/api/auth"
	"github.com/luma/flightrecorder/services"
)

func registerAdminRoutes(adminGroup *route.RouterGroup, adminAuth services.AdminAuth, adminSvc services.Admin, log *slog.Logger, secureCookies bool) {
	adminGroup.POST("/auth/dev-login", makeAdminDevLogin(adminAuth, secureCookies))
	adminGroup.POST("/auth/logout", makeAdminLogout(secureCookies))
	adminGroup.GET("/auth/me", makeAdminMe(adminAuth))

	protected := adminGroup.Group("", auth.RequireAdmin(adminAuth, log))
	protected.GET("/summary", makeAdminSummary(adminSvc))
	protected.GET("/events", makeAdminEvents(adminSvc))
	protected.GET("/players/:player_id/trace", makeAdminPlayerTrace(adminSvc))
	protected.GET("/heatmap/regions", makeAdminRegionHeatmap(adminSvc))
	protected.GET("/heatmap/zones", makeAdminZoneHeatmap(adminSvc))
	protected.GET("/funnels", makeAdminFunnels(adminSvc))
	protected.GET("/reports", makeAdminReports(adminSvc))
	protected.GET("/reports/:report_id", makeAdminReportDetail(adminSvc))
	protected.GET("/reports/:report_id/screenshot", makeAdminReportScreenshot(adminSvc))
	protected.PATCH("/reports/:report_id", makeAdminReportUpdate(adminSvc))
	protected.GET("/event-types", makeAdminEventTypes(adminSvc))
	protected.GET("/projects", makeAdminProjects(adminSvc))
	protected.POST("/projects", makeAdminCreateProject(adminSvc))
	protected.GET("/settings", makeAdminSettings(adminSvc))
	protected.POST("/settings/ingest-tokens", makeAdminCreateIngestToken(adminSvc))
	protected.PATCH("/settings/ingest-tokens/:token_id", makeAdminSetIngestTokenEnabled(adminSvc))
}

func makeAdminDevLogin(adminAuth services.AdminAuth, secureCookies bool) app.HandlerFunc {
	type request struct {
		Email string `json:"email"`
	}
	return func(ctx context.Context, c *app.RequestContext) {
		var req request
		if err := decodeJSONBody(c, &req); err != nil {
			writeServiceError(c, fmt.Errorf("%w: %v", services.ErrBadRequest, err))
			return
		}
		token, session, err := adminAuth.IssueDevSession(req.Email)
		if err != nil {
			c.JSON(consts.StatusUnauthorized, map[string]string{"error": "admin authentication failed"})
			return
		}
		setAdminCookie(c, token, 12*60*60, secureCookies)
		c.JSON(consts.StatusOK, map[string]any{
			"user": map[string]string{
				"email": session.Email,
			},
			"expires_at": session.ExpiresAt,
		})
	}
}

func makeAdminLogout(secureCookies bool) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		_ = ctx
		setAdminCookie(c, "", -1, secureCookies)
		c.JSON(consts.StatusOK, map[string]bool{"ok": true})
	}
}

func makeAdminMe(adminAuth services.AdminAuth) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		_ = ctx
		session, err := adminAuth.ValidateSession(string(c.Cookie(auth.AdminSessionCookieName)))
		if err != nil {
			c.JSON(consts.StatusUnauthorized, map[string]string{"error": "admin authentication required"})
			return
		}
		c.JSON(consts.StatusOK, map[string]any{
			"user": map[string]string{
				"email": session.Email,
			},
			"expires_at": session.ExpiresAt,
		})
	}
}

func makeAdminSummary(adminSvc services.Admin) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		filter, err := adminTimeFilter(c)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		resp, err := adminSvc.Summary(ctx, filter)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(consts.StatusOK, resp)
	}
}

func makeAdminEvents(adminSvc services.Admin) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		timeFilter, err := adminTimeFilter(c)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		events, err := adminSvc.ListEvents(ctx, services.EventListFilter{
			TimeProjectFilter: timeFilter,
			EventType:         queryPtr(c, "event_type"),
			RegionID:          queryPtr(c, "region_id"),
			ZoneID:            queryPtr(c, "zone_id"),
			PlayerID:          queryPtr(c, "player_id"),
			GameVersion:       queryPtr(c, "game_version"),
			BuildChannel:      queryPtr(c, "build_channel"),
			FieldKey:          queryPtr(c, "field_key"),
			FieldValue:        queryPtr(c, "field_value"),
			Limit:             services.ParseLimit(query(c, "limit"), 100),
			Offset:            services.ParseOffset(query(c, "offset")),
		})
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(consts.StatusOK, map[string]any{"events": events})
	}
}

func makeAdminPlayerTrace(adminSvc services.Admin) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		trace, err := adminSvc.PlayerTrace(ctx, query(c, "project_id"), c.Param("player_id"), services.ParseLimit(query(c, "limit"), 500))
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(consts.StatusOK, map[string]any{"events": trace})
	}
}

func makeAdminRegionHeatmap(adminSvc services.Admin) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		timeFilter, err := adminTimeFilter(c)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		cells, err := adminSvc.RegionHeatmap(ctx, services.HeatmapFilter{
			TimeProjectFilter: timeFilter,
			EventType:         queryPtr(c, "event_type"),
			GameVersion:       queryPtr(c, "game_version"),
			BuildChannel:      queryPtr(c, "build_channel"),
			FieldKey:          queryPtr(c, "field_key"),
			FieldValue:        queryPtr(c, "field_value"),
		})
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(consts.StatusOK, map[string]any{"cells": cells})
	}
}

func makeAdminZoneHeatmap(adminSvc services.Admin) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		timeFilter, err := adminTimeFilter(c)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		cells, err := adminSvc.ZoneHeatmap(ctx, services.ZoneHeatmapFilter{
			HeatmapFilter: services.HeatmapFilter{
				TimeProjectFilter: timeFilter,
				EventType:         queryPtr(c, "event_type"),
				GameVersion:       queryPtr(c, "game_version"),
				BuildChannel:      queryPtr(c, "build_channel"),
				FieldKey:          queryPtr(c, "field_key"),
				FieldValue:        queryPtr(c, "field_value"),
			},
			RegionID: query(c, "region_id"),
			ZoneID:   queryPtr(c, "zone_id"),
			CellM:    float64(services.ParseLimit(query(c, "cell_m"), 300)),
		})
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(consts.StatusOK, map[string]any{"cells": cells})
	}
}

func makeAdminFunnels(adminSvc services.Admin) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		filter, err := adminTimeFilter(c)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		resp, err := adminSvc.Funnels(ctx, filter)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(consts.StatusOK, resp)
	}
}

func makeAdminReports(adminSvc services.Admin) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		reports, err := adminSvc.ListReports(ctx, services.ReportListFilter{
			ProjectID: query(c, "project_id"),
			Status:    queryPtr(c, "status"),
			Label:     queryPtr(c, "label"),
			Limit:     services.ParseLimit(query(c, "limit"), 100),
			Offset:    services.ParseOffset(query(c, "offset")),
		})
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(consts.StatusOK, map[string]any{"reports": reports})
	}
}

func makeAdminReportDetail(adminSvc services.Admin) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		report, err := adminSvc.GetReport(ctx, query(c, "project_id"), c.Param("report_id"))
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(consts.StatusOK, report)
	}
}

func makeAdminReportScreenshot(adminSvc services.Admin) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		screenshot, err := adminSvc.ReportScreenshot(ctx, query(c, "project_id"), c.Param("report_id"))
		if err != nil {
			writeServiceError(c, err)
			return
		}
		if screenshot.RedirectURL != "" {
			c.Redirect(consts.StatusTemporaryRedirect, []byte(screenshot.RedirectURL))
			return
		}
		contentType := screenshot.ContentType
		if contentType == "" {
			contentType = "image/png"
		}
		c.SetContentType(contentType)
		c.Response.Header.Set("Cache-Control", "private, max-age=300")
		c.SetStatusCode(consts.StatusOK)
		c.Response.SetBody(screenshot.Data)
	}
}

func makeAdminReportUpdate(adminSvc services.Admin) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req services.UpdateReportRequest
		if err := decodeJSONBody(c, &req); err != nil {
			writeServiceError(c, fmt.Errorf("%w: %v", services.ErrBadRequest, err))
			return
		}
		report, err := adminSvc.UpdateReport(ctx, query(c, "project_id"), c.Param("report_id"), req)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(consts.StatusOK, report)
	}
}

func makeAdminEventTypes(adminSvc services.Admin) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		eventTypes, err := adminSvc.EventTypes(ctx, query(c, "project_id"))
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(consts.StatusOK, map[string]any{"event_types": eventTypes})
	}
}

func makeAdminProjects(adminSvc services.Admin) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		projects, err := adminSvc.ListProjects(ctx)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(consts.StatusOK, map[string]any{"projects": projects})
	}
}

func makeAdminCreateProject(adminSvc services.Admin) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req services.CreateProjectRequest
		if err := decodeJSONBody(c, &req); err != nil {
			writeServiceError(c, fmt.Errorf("%w: %v", services.ErrBadRequest, err))
			return
		}
		project, err := adminSvc.CreateProject(ctx, req)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(consts.StatusOK, project)
	}
}

func makeAdminSettings(adminSvc services.Admin) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		settings, err := adminSvc.Settings(ctx, query(c, "project_id"))
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(consts.StatusOK, settings)
	}
}

func makeAdminCreateIngestToken(adminSvc services.Admin) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req services.CreateIngestTokenRequest
		if err := decodeJSONBody(c, &req); err != nil {
			writeServiceError(c, fmt.Errorf("%w: %v", services.ErrBadRequest, err))
			return
		}
		token, err := adminSvc.CreateIngestToken(ctx, query(c, "project_id"), req)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(consts.StatusOK, token)
	}
}

func makeAdminSetIngestTokenEnabled(adminSvc services.Admin) app.HandlerFunc {
	type request struct {
		Enabled bool `json:"enabled"`
	}
	return func(ctx context.Context, c *app.RequestContext) {
		var req request
		if err := decodeJSONBody(c, &req); err != nil {
			writeServiceError(c, fmt.Errorf("%w: %v", services.ErrBadRequest, err))
			return
		}
		token, err := adminSvc.SetIngestTokenEnabled(ctx, query(c, "project_id"), c.Param("token_id"), req.Enabled)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(consts.StatusOK, token)
	}
}

func adminTimeFilter(c *app.RequestContext) (services.TimeProjectFilter, error) {
	return services.ParseTimeRange(query(c, "project_id"), query(c, "from"), query(c, "to"))
}

func queryPtr(c *app.RequestContext, name string) *string {
	return services.OptionalString(query(c, name))
}

func query(c *app.RequestContext, name string) string {
	return string(c.Query(name))
}

func setAdminCookie(c *app.RequestContext, value string, maxAge int, secure bool) {
	c.SetCookie(auth.AdminSessionCookieName, value, maxAge, "/", "", protocol.CookieSameSiteLaxMode, secure, true)
}
