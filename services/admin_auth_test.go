package services

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("admin auth", func() {
	authService := func(devLogin bool) AdminAuth {
		return NewAdminAuthService(AdminAuthOptions{
			SessionSecret:   "test-secret",
			AllowedEmails:   "admin@example.com, second@example.com",
			DevLogin:        devLogin,
			SessionDuration: 5 * time.Minute,
		})
	}

	It("issues and validates signed dev sessions for allowlisted emails", func() {
		token, session, err := authService(true).IssueDevSession(" Admin@Example.com ")

		Expect(err).ToNot(HaveOccurred())
		Expect(token).ToNot(BeEmpty())
		Expect(session.Email).To(Equal("admin@example.com"))

		validated, err := authService(true).ValidateSession(token)

		Expect(err).ToNot(HaveOccurred())
		Expect(validated.Email).To(Equal("admin@example.com"))
	})

	It("rejects non-allowlisted emails", func() {
		_, _, err := authService(true).IssueDevSession("intruder@example.com")

		Expect(err).To(MatchError(ErrAdminAuthFailed))
	})

	It("rejects dev sessions when dev login is disabled", func() {
		_, _, err := authService(false).IssueDevSession("admin@example.com")

		Expect(err).To(MatchError(ErrAdminAuthFailed))
	})

	It("rejects tampered session tokens", func() {
		token, _, err := authService(true).IssueDevSession("admin@example.com")
		Expect(err).ToNot(HaveOccurred())

		parts := strings.Split(token, ".")
		Expect(parts).To(HaveLen(2))

		_, err = authService(true).ValidateSession(parts[0] + ".not-the-signature")

		Expect(err).To(MatchError(ErrAdminAuthFailed))
	})
})
