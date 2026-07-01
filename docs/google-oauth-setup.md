# Google OAuth Setup

flightrecorder uses two auth mechanisms:

- Admin web UI: Google OAuth followed by the signed `flightrecorder_admin` browser cookie.
- Reporter ingestion: project-scoped bearer tokens sent with `Authorization: Bearer ...`.

OAuth callbacks terminate at the API route:

```text
/api/admin/v1/auth/google/callback
```

## Google Cloud

1. Open Google Cloud Console and select the project that should own the OAuth app.
2. Configure the OAuth consent screen.
3. Create an OAuth 2.0 Client ID with application type `Web application`.
4. Configure these OAuth scopes:

```text
openid
email
profile
```

`openid` is required so the callback receives a verifiable ID token. `email` and `profile` provide the verified email, name, and profile image used by the admin UI.

5. Add authorized redirect URIs:

```text
https://telemetry.no8wire.gg/api/admin/v1/auth/google/callback
http://localhost:8080/api/admin/v1/auth/google/callback
```

6. Add authorized JavaScript origins if Google asks for them:

```text
https://telemetry.no8wire.gg
http://localhost:8080
```

## Environment

Production should set:

```text
ENVIRONMENT=production
API_DOMAIN=telemetry.no8wire.gg
WEB_BASE_URL=https://telemetry.no8wire.gg
GOOGLE_OAUTH_CLIENT_ID=...
GOOGLE_OAUTH_CLIENT_SECRET=...
ADMIN_ALLOWED_DOMAINS=no8wire.gg
ADMIN_BOOTSTRAP_EMAIL=you@no8wire.gg
ADMIN_SESSION_SECRET=<strong random value>
ADMIN_DEV_LOGIN=false
```

`ADMIN_BOOTSTRAP_EMAIL` must match `ADMIN_ALLOWED_DOMAINS`. It can create the first enabled admin user without an invitation only while there are zero admin users. Once any admin user exists, the bootstrap path is disabled.

All later users must sign in with Google, match `ADMIN_ALLOWED_DOMAINS`, and accept a valid invitation.

`GOOGLE_OAUTH_REDIRECT_URL` is optional. If unset, the service derives it from `API_DOMAIN`.

If the Google OAuth variables and admin domain settings are omitted in production, the service still starts, but the admin web UI has no enabled login method. This allows reporter ingestion to stay online while OAuth secrets are being provisioned. `ADMIN_DEV_LOGIN=true` is still rejected in production.

## Local Development

The default local flow can use dev-login:

```text
ADMIN_DEV_LOGIN=true
```

To test real Google OAuth locally, register this redirect URI in Google Cloud:

```text
http://localhost:8080/api/admin/v1/auth/google/callback
```

Then set:

```text
GOOGLE_OAUTH_CLIENT_ID=...
GOOGLE_OAUTH_CLIENT_SECRET=...
ADMIN_ALLOWED_DOMAINS=example.com
ADMIN_BOOTSTRAP_EMAIL=admin@example.com
```

## Operations

After the bootstrap admin signs in, use the `Users` tab to:

- Invite users by email.
- Copy the one-time invitation code.
- View active invitations.
- Delete active invitations.
- Enable or disable admin users.

Invitation codes expire after 48 hours and are stored only as hashes.

## Troubleshooting

- `redirect_uri_mismatch`: confirm the callback URL in Google Cloud exactly matches the API callback route.
- Login redirects to an error page: check the `reason` query parameter and confirm the Google email domain is allowed.
- Invite code rejected: confirm the invitation is active, not expired, and was created for the same Google email.
- `/auth/me` returns `401`: the admin cookie may be expired, the user may be disabled, or the email domain may no longer be allowed.
