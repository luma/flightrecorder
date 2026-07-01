package services

import (
	"context"
	"fmt"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/idtoken"
)

type GoogleOAuth interface {
	AuthCodeURL(state string) string
	Exchange(ctx context.Context, code string) (OAuthIdentity, error)
	Enabled() bool
}

type GoogleOAuthOptions struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

type googleOAuthService struct {
	config GoogleOAuthOptions
	oauth  *oauth2.Config
}

func NewGoogleOAuthService(opts GoogleOAuthOptions) GoogleOAuth {
	if opts.ClientID == "" || opts.ClientSecret == "" || opts.RedirectURL == "" {
		return disabledGoogleOAuth{}
	}
	return &googleOAuthService{
		config: opts,
		oauth: &oauth2.Config{
			ClientID:     opts.ClientID,
			ClientSecret: opts.ClientSecret,
			RedirectURL:  opts.RedirectURL,
			Scopes:       []string{"openid", "email", "profile"},
			Endpoint:     google.Endpoint,
		},
	}
}

func (s *googleOAuthService) Enabled() bool {
	return true
}

func (s *googleOAuthService) AuthCodeURL(state string) string {
	return s.oauth.AuthCodeURL(state, oauth2.AccessTypeOnline)
}

func (s *googleOAuthService) Exchange(ctx context.Context, code string) (OAuthIdentity, error) {
	token, err := s.oauth.Exchange(ctx, code)
	if err != nil {
		return OAuthIdentity{}, err
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return OAuthIdentity{}, fmt.Errorf("google oauth response missing id_token")
	}
	payload, err := idtoken.Validate(ctx, rawIDToken, s.config.ClientID)
	if err != nil {
		return OAuthIdentity{}, err
	}
	email, _ := payload.Claims["email"].(string)
	name, _ := payload.Claims["name"].(string)
	picture, _ := payload.Claims["picture"].(string)
	emailVerified, _ := payload.Claims["email_verified"].(bool)
	if payload.Subject == "" || email == "" || !emailVerified {
		return OAuthIdentity{}, ErrAdminAuthFailed
	}
	return normalizeOAuthIdentity(OAuthIdentity{
		Provider: adminSessionProviderGoogle,
		Subject:  payload.Subject,
		Email:    email,
		Name:     name,
		Picture:  picture,
	}), nil
}

type disabledGoogleOAuth struct{}

func (disabledGoogleOAuth) Enabled() bool {
	return false
}

func (disabledGoogleOAuth) AuthCodeURL(string) string {
	return ""
}

func (disabledGoogleOAuth) Exchange(context.Context, string) (OAuthIdentity, error) {
	return OAuthIdentity{}, ErrAdminAuthFailed
}
