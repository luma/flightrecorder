package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"github.com/luma/flightrecorder/api/auth"
	"github.com/luma/flightrecorder/services"
)

func registerMCPRoutes(h *server.Hertz, adminAuth services.AdminAuth, adminSvc services.Admin, agentAuth services.AgentAuth, mcpOAuth services.MCPOAuth, log *slog.Logger, secureCookies bool, apiBaseURL string) {
	h.GET("/.well-known/oauth-authorization-server", makeMCPAuthorizationServerMetadata(mcpOAuth))
	h.GET("/.well-known/openid-configuration", makeMCPAuthorizationServerMetadata(mcpOAuth))
	h.GET("/.well-known/oauth-protected-resource", makeMCPProtectedResourceMetadata(mcpOAuth))
	h.GET("/.well-known/oauth-protected-resource/mcp", makeMCPProtectedResourceMetadata(mcpOAuth))
	h.GET("/api/mcp/oauth/authorize", makeMCPAuthorize(adminAuth, mcpOAuth))
	h.POST("/api/mcp/oauth/register", makeMCPRegisterClient(mcpOAuth))
	h.POST("/api/mcp/oauth/token", makeMCPToken(mcpOAuth))

	consent := h.Group("/api/mcp/oauth", auth.RequireAdmin(adminAuth, log))
	consent.GET("/consent", makeMCPConsentDetails(mcpOAuth))
	consent.POST("/consent", makeMCPConfirmConsent(mcpOAuth))

	mcp := h.Group("", auth.RequireMCPAgent(agentAuth, log, strings.TrimRight(apiBaseURL, "/")+"/.well-known/oauth-protected-resource/mcp"))
	mcp.POST("/mcp", makeMCPJSONRPC(adminSvc))
}

func makeMCPAuthorizationServerMetadata(mcpOAuth services.MCPOAuth) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		_ = ctx
		c.JSON(consts.StatusOK, mcpOAuth.ServerMetadata())
	}
}

func makeMCPProtectedResourceMetadata(mcpOAuth services.MCPOAuth) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		_ = ctx
		c.JSON(consts.StatusOK, mcpOAuth.ProtectedResourceMetadata())
	}
}

func makeMCPRegisterClient(mcpOAuth services.MCPOAuth) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req services.MCPClientRegistrationRequest
		if err := json.Unmarshal(c.Request.Body(), &req); err != nil {
			writeServiceError(c, fmt.Errorf("%w: %v", services.ErrBadRequest, err))
			return
		}
		resp, err := mcpOAuth.RegisterClient(ctx, req)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(consts.StatusCreated, resp)
	}
}

func makeMCPAuthorize(adminAuth services.AdminAuth, mcpOAuth services.MCPOAuth) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		token := string(c.Cookie(auth.AdminSessionCookieName))
		if _, err := adminAuth.ValidateSession(ctx, token); err != nil {
			returnPath := string(c.URI().Path())
			if rawQuery := string(c.URI().QueryString()); rawQuery != "" {
				returnPath += "?" + rawQuery
			}
			c.Redirect(consts.StatusTemporaryRedirect, []byte("/login?return_path="+url.QueryEscape(returnPath)))
			return
		}
		prepared, err := mcpOAuth.PrepareAuthorization(ctx, services.MCPAuthorizationRequest{
			ResponseType:        query(c, "response_type"),
			ClientID:            query(c, "client_id"),
			RedirectURI:         query(c, "redirect_uri"),
			CodeChallenge:       query(c, "code_challenge"),
			CodeChallengeMethod: query(c, "code_challenge_method"),
			State:               query(c, "state"),
			Resource:            query(c, "resource"),
			Scope:               query(c, "scope"),
		})
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.Redirect(consts.StatusTemporaryRedirect, []byte("/mcp/consent?request="+url.QueryEscape(prepared.RequestToken)))
	}
}

func makeMCPConsentDetails(mcpOAuth services.MCPOAuth) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		details, err := mcpOAuth.ConsentDetails(ctx, query(c, "request"))
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(consts.StatusOK, details)
	}
}

func makeMCPConfirmConsent(mcpOAuth services.MCPOAuth) app.HandlerFunc {
	type response struct {
		RedirectURI string `json:"redirect_uri"`
	}
	return func(ctx context.Context, c *app.RequestContext) {
		session, ok := services.AdminSessionFromContext(ctx)
		if !ok {
			c.JSON(consts.StatusUnauthorized, map[string]string{"error": "admin authentication required"})
			return
		}
		var req services.MCPConsentRequest
		if err := decodeJSONBody(c, &req); err != nil {
			writeServiceError(c, fmt.Errorf("%w: %v", services.ErrBadRequest, err))
			return
		}
		redirectURI, err := mcpOAuth.ConfirmConsent(ctx, query(c, "request"), session, req)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(consts.StatusOK, response{RedirectURI: redirectURI})
	}
}

func makeMCPToken(mcpOAuth services.MCPOAuth) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		resp, err := mcpOAuth.ExchangeCode(ctx, services.MCPTokenRequest{
			GrantType:    formValue(c, "grant_type"),
			ClientID:     formValue(c, "client_id"),
			Code:         formValue(c, "code"),
			RedirectURI:  formValue(c, "redirect_uri"),
			CodeVerifier: formValue(c, "code_verifier"),
			Resource:     formValue(c, "resource"),
		})
		if err != nil {
			if errors.Is(err, services.ErrAgentAuthFailed) {
				c.JSON(consts.StatusBadRequest, map[string]string{"error": "invalid_grant"})
				return
			}
			writeServiceError(c, err)
			return
		}
		c.JSON(consts.StatusOK, resp)
	}
}

func formValue(c *app.RequestContext, name string) string {
	return strings.TrimSpace(string(c.PostArgs().Peek(name)))
}
