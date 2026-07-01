package services

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("admin auth", func() {
	authService := func(devLogin bool) AdminAuth {
		return NewAdminAuthService(AdminAuthOptions{
			SessionSecret:   "test-secret",
			AllowedDomains:  "example.com",
			DevLogin:        devLogin,
			SessionDuration: 5 * time.Minute,
		})
	}

	It("issues and validates signed dev sessions for allowed domains", func() {
		token, session, err := authService(true).IssueDevSession(context.Background(), " Admin@Example.com ")

		Expect(err).ToNot(HaveOccurred())
		Expect(token).ToNot(BeEmpty())
		Expect(session.Email).To(Equal("admin@example.com"))

		validated, err := authService(true).ValidateSession(context.Background(), token)

		Expect(err).ToNot(HaveOccurred())
		Expect(validated.Email).To(Equal("admin@example.com"))
	})

	It("rejects non-allowed domains", func() {
		_, _, err := authService(true).IssueDevSession(context.Background(), "intruder@evil.test")

		Expect(err).To(MatchError(ErrAdminAuthFailed))
	})

	It("rejects dev sessions when dev login is disabled", func() {
		_, _, err := authService(false).IssueDevSession(context.Background(), "admin@example.com")

		Expect(err).To(MatchError(ErrAdminAuthFailed))
	})

	It("rejects tampered session tokens", func() {
		token, _, err := authService(true).IssueDevSession(context.Background(), "admin@example.com")
		Expect(err).ToNot(HaveOccurred())

		parts := strings.Split(token, ".")
		Expect(parts).To(HaveLen(2))

		_, err = authService(true).ValidateSession(context.Background(), parts[0]+".not-the-signature")

		Expect(err).To(MatchError(ErrAdminAuthFailed))
	})

	It("bootstraps the first Google user only when the domain is allowed", func() {
		store := newMemoryAdminUserStore()
		auth := NewAdminAuthService(AdminAuthOptions{
			SessionSecret:   "test-secret",
			AllowedDomains:  "example.com",
			BootstrapEmail:  "admin@example.com",
			SessionDuration: 5 * time.Minute,
			UserStore:       store,
			GoogleEnabled:   true,
		})

		result, err := auth.IssueOAuthSession(context.Background(), OAuthIdentity{
			Subject: "google-1",
			Email:   "Admin@Example.com",
			Name:    "Admin",
		})

		Expect(err).ToNot(HaveOccurred())
		Expect(result.Status).To(Equal(AdminOAuthStatusAuthenticated))
		Expect(result.Token).ToNot(BeEmpty())
		Expect(store.count).To(Equal(1))
	})

	It("does not bootstrap outside allowed domains", func() {
		auth := NewAdminAuthService(AdminAuthOptions{
			SessionSecret:   "test-secret",
			AllowedDomains:  "example.com",
			BootstrapEmail:  "admin@example.com",
			SessionDuration: 5 * time.Minute,
			UserStore:       newMemoryAdminUserStore(),
			GoogleEnabled:   true,
		})

		result, err := auth.IssueOAuthSession(context.Background(), OAuthIdentity{
			Subject: "google-1",
			Email:   "admin@evil.test",
		})

		Expect(err).ToNot(HaveOccurred())
		Expect(result.Status).To(Equal(AdminOAuthStatusDomainDenied))
	})

	It("revalidates enabled users when validating existing sessions", func() {
		store := newMemoryAdminUserStore()
		auth := NewAdminAuthService(AdminAuthOptions{
			SessionSecret:   "test-secret",
			AllowedDomains:  "example.com",
			DevLogin:        true,
			SessionDuration: 5 * time.Minute,
			UserStore:       store,
		})
		token, session, err := auth.IssueDevSession(context.Background(), "admin@example.com")
		Expect(err).ToNot(HaveOccurred())
		user := store.users[session.Email]
		user.Enabled = false
		store.users[session.Email] = user

		_, err = auth.ValidateSession(context.Background(), token)

		Expect(err).To(MatchError(ErrAdminAuthFailed))
	})

	It("issues an OAuth session with profile data for an existing enabled user", func() {
		store := newMemoryAdminUserStore()
		store.seedEnabledUser(OAuthIdentity{Subject: "google-1", Email: "user@example.com", Name: "Old Name"})
		auth := oauthAuthService(store)

		result, err := auth.IssueOAuthSession(context.Background(), OAuthIdentity{
			Subject: "google-1",
			Email:   "user@example.com",
			Name:    "New Name",
			Picture: "https://pic.example/avatar.png",
		})

		Expect(err).ToNot(HaveOccurred())
		Expect(result.Status).To(Equal(AdminOAuthStatusAuthenticated))
		Expect(result.Session.Email).To(Equal("user@example.com"))
		Expect(result.Session.Name).To(Equal("New Name"))
		Expect(result.Session.Picture).To(Equal("https://pic.example/avatar.png"))
	})

	It("finds an existing user by Google subject even when the email changed", func() {
		store := newMemoryAdminUserStore()
		store.seedEnabledUser(OAuthIdentity{Subject: "google-1", Email: "old@example.com", Name: "User"})
		auth := oauthAuthService(store)

		result, err := auth.IssueOAuthSession(context.Background(), OAuthIdentity{
			Subject: "google-1",
			Email:   "new@example.com",
		})

		Expect(err).ToNot(HaveOccurred())
		Expect(result.Status).To(Equal(AdminOAuthStatusAuthenticated))
	})

	It("rejects a disabled user during OAuth login", func() {
		store := newMemoryAdminUserStore()
		user := store.seedEnabledUser(OAuthIdentity{Subject: "google-1", Email: "user@example.com"})
		user.Enabled = false
		store.users[user.Email] = user
		auth := oauthAuthService(store)

		result, err := auth.IssueOAuthSession(context.Background(), OAuthIdentity{
			Subject: "google-1",
			Email:   "user@example.com",
		})

		Expect(err).ToNot(HaveOccurred())
		Expect(result.Status).To(Equal(AdminOAuthStatusDisabled))
	})

	It("returns pending invite for unknown domain-allowed users", func() {
		store := newMemoryAdminUserStore()
		store.seedEnabledUser(OAuthIdentity{Subject: "google-1", Email: "existing@example.com"})
		auth := oauthAuthService(store)

		result, err := auth.IssueOAuthSession(context.Background(), OAuthIdentity{
			Subject: "google-2",
			Email:   "newcomer@example.com",
		})

		Expect(err).ToNot(HaveOccurred())
		Expect(result.Status).To(Equal(AdminOAuthStatusPendingInvite))
		Expect(result.PendingToken).ToNot(BeEmpty())
	})

	It("does not bootstrap once a user already exists", func() {
		store := newMemoryAdminUserStore()
		store.seedEnabledUser(OAuthIdentity{Subject: "google-1", Email: "existing@example.com"})
		auth := NewAdminAuthService(AdminAuthOptions{
			SessionSecret:   "test-secret",
			AllowedDomains:  "example.com",
			BootstrapEmail:  "admin@example.com",
			SessionDuration: 5 * time.Minute,
			UserStore:       store,
			GoogleEnabled:   true,
		})

		result, err := auth.IssueOAuthSession(context.Background(), OAuthIdentity{
			Subject: "google-2",
			Email:   "admin@example.com",
		})

		Expect(err).ToNot(HaveOccurred())
		Expect(result.Status).To(Equal(AdminOAuthStatusPendingInvite))
	})

	It("accepts a valid invitation for the pending email", func() {
		store := newMemoryAdminUserStore()
		auth := oauthAuthService(store)
		pending, err := auth.IssueOAuthSession(context.Background(), OAuthIdentity{
			Subject: "google-1",
			Email:   "invitee@example.com",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(pending.Status).To(Equal(AdminOAuthStatusPendingInvite))

		token, hash, err := NewAdminInviteToken()
		Expect(err).ToNot(HaveOccurred())
		store.invites["invitee@example.com"] = hash

		sessionToken, session, err := auth.AcceptInvitation(context.Background(), pending.PendingToken, token)

		Expect(err).ToNot(HaveOccurred())
		Expect(sessionToken).ToNot(BeEmpty())
		Expect(session.Email).To(Equal("invitee@example.com"))
		Expect(store.count).To(Equal(1))
	})

	It("rejects an incorrect invitation code", func() {
		store := newMemoryAdminUserStore()
		auth := oauthAuthService(store)
		pending, err := auth.IssueOAuthSession(context.Background(), OAuthIdentity{
			Subject: "google-1",
			Email:   "invitee@example.com",
		})
		Expect(err).ToNot(HaveOccurred())

		_, correctHash, err := NewAdminInviteToken()
		Expect(err).ToNot(HaveOccurred())
		store.invites["invitee@example.com"] = correctHash

		_, _, err = auth.AcceptInvitation(context.Background(), pending.PendingToken, "fr_invite_wrong")

		Expect(err).To(MatchError(ErrAdminAuthFailed))
		Expect(store.count).To(Equal(0))
	})

	It("rejects invitation acceptance with a missing pending token", func() {
		auth := oauthAuthService(newMemoryAdminUserStore())

		_, _, err := auth.AcceptInvitation(context.Background(), "", "fr_invite_anything")

		Expect(err).To(MatchError(ErrAdminAuthFailed))
	})

	It("signs and validates OAuth state carrying a return path", func() {
		auth := authService(true)

		token, err := auth.IssueOAuthState("/dashboard")
		Expect(err).ToNot(HaveOccurred())

		returnPath, err := auth.ValidateOAuthState(token)
		Expect(err).ToNot(HaveOccurred())
		Expect(returnPath).To(Equal("/dashboard"))
	})

	It("rejects tampered OAuth state", func() {
		auth := authService(true)
		token, err := auth.IssueOAuthState("/dashboard")
		Expect(err).ToNot(HaveOccurred())
		parts := strings.Split(token, ".")
		Expect(parts).To(HaveLen(2))

		_, err = auth.ValidateOAuthState(parts[0] + ".not-the-signature")

		Expect(err).To(MatchError(ErrAdminAuthFailed))
	})

	It("rejects expired OAuth state", func() {
		svc := authService(true).(*adminAuthService)
		token, err := svc.signJSON(adminStateToken{
			ReturnPath: "/dashboard",
			ExpiresAt:  time.Now().UTC().Add(-time.Minute),
		})
		Expect(err).ToNot(HaveOccurred())

		_, err = svc.ValidateOAuthState(token)

		Expect(err).To(MatchError(ErrAdminAuthFailed))
	})
})

func oauthAuthService(store AdminUserStore) AdminAuth {
	return NewAdminAuthService(AdminAuthOptions{
		SessionSecret:   "test-secret",
		AllowedDomains:  "example.com",
		SessionDuration: 5 * time.Minute,
		UserStore:       store,
		GoogleEnabled:   true,
	})
}

func (s *memoryAdminUserStore) seedEnabledUser(identity OAuthIdentity) AdminUserRecord {
	user, _ := s.CreateUser(context.Background(), normalizeOAuthIdentity(identity))
	return user
}

type memoryAdminUserStore struct {
	users   map[string]AdminUserRecord
	count   int
	invites map[string]string // normalized email -> active token hash
}

func newMemoryAdminUserStore() *memoryAdminUserStore {
	return &memoryAdminUserStore{
		users:   map[string]AdminUserRecord{},
		invites: map[string]string{},
	}
}

func (s *memoryAdminUserStore) CountUsers(context.Context) (int64, error) {
	return int64(s.count), nil
}

func (s *memoryAdminUserStore) FindUserByEmail(_ context.Context, email string) (AdminUserRecord, bool, error) {
	user, ok := s.users[email]
	return user, ok, nil
}

func (s *memoryAdminUserStore) FindUserBySubject(_ context.Context, subject string) (AdminUserRecord, bool, error) {
	for _, user := range s.users {
		if user.OAuthSubject == subject {
			return user, true, nil
		}
	}
	return AdminUserRecord{}, false, nil
}

func (s *memoryAdminUserStore) CreateUser(_ context.Context, identity OAuthIdentity) (AdminUserRecord, error) {
	if _, ok := s.users[identity.Email]; ok {
		return AdminUserRecord{}, errors.New("duplicate user")
	}
	now := time.Now().UTC()
	user := AdminUserRecord{
		ID:           uuid.New(),
		Email:        identity.Email,
		OAuthSubject: identity.Subject,
		Role:         "admin",
		Enabled:      true,
		Name:         identity.Name,
		Picture:      identity.Picture,
		Provider:     identity.Provider,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.users[user.Email] = user
	s.count++
	return user, nil
}

func (s *memoryAdminUserStore) RefreshUserLogin(_ context.Context, userID uuid.UUID, identity OAuthIdentity) (AdminUserRecord, error) {
	for email, user := range s.users {
		if user.ID == userID {
			user.Name = identity.Name
			user.Picture = identity.Picture
			user.Provider = identity.Provider
			user.OAuthSubject = identity.Subject
			s.users[email] = user
			return user, nil
		}
	}
	return AdminUserRecord{}, errors.New("missing user")
}

func (s *memoryAdminUserStore) AcceptInvitationAndCreateUser(ctx context.Context, identity OAuthIdentity, tokenHash string) (AdminUserRecord, error) {
	active, ok := s.invites[identity.Email]
	if !ok || active != tokenHash {
		return AdminUserRecord{}, pgx.ErrNoRows
	}
	delete(s.invites, identity.Email)
	return s.CreateUser(ctx, identity)
}
