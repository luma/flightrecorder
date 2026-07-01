package env_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/luma/flightrecorder/env"
)

func productionConfig() env.Config {
	return env.Config{
		Environment:         "production",
		AdminAllowedDomains: "example.com",
		AdminBootstrapEmail: "admin@example.com",
		AdminSessionSecret:  "a-strong-production-secret",
		AdminDevLogin:       false,
		GoogleOAuthClientID: "client-id",
		GoogleOAuthSecret:   "client-secret",
	}
}

var _ = Describe("Config.Validate()", func() {
	It("accepts a complete production config", func() {
		cfg := productionConfig()
		Expect(cfg.Validate()).To(Succeed())
	})

	It("rejects missing ADMIN_ALLOWED_DOMAINS in production", func() {
		cfg := productionConfig()
		cfg.AdminAllowedDomains = ""
		Expect(cfg.Validate()).To(MatchError(ContainSubstring("ADMIN_ALLOWED_DOMAINS")))
	})

	It("rejects missing ADMIN_BOOTSTRAP_EMAIL in production", func() {
		cfg := productionConfig()
		cfg.AdminBootstrapEmail = ""
		Expect(cfg.Validate()).To(MatchError(ContainSubstring("ADMIN_BOOTSTRAP_EMAIL is required")))
	})

	It("rejects a bootstrap email outside the allowed domains", func() {
		cfg := productionConfig()
		cfg.AdminBootstrapEmail = "admin@other.test"
		Expect(cfg.Validate()).To(MatchError(ContainSubstring("must match ADMIN_ALLOWED_DOMAINS")))
	})

	It("rejects the default admin session secret in production", func() {
		cfg := productionConfig()
		cfg.AdminSessionSecret = "dev-admin-session-secret-change-me"
		Expect(cfg.Validate()).To(MatchError(ContainSubstring("ADMIN_SESSION_SECRET")))
	})

	It("rejects ADMIN_DEV_LOGIN=true in production", func() {
		cfg := productionConfig()
		cfg.AdminDevLogin = true
		Expect(cfg.Validate()).To(MatchError(ContainSubstring("ADMIN_DEV_LOGIN")))
	})

	It("rejects missing Google OAuth credentials in production", func() {
		cfg := productionConfig()
		cfg.GoogleOAuthClientID = ""
		cfg.GoogleOAuthSecret = ""
		err := cfg.Validate()
		Expect(err).To(MatchError(ContainSubstring("GOOGLE_OAUTH_CLIENT_ID")))
		Expect(err).To(MatchError(ContainSubstring("GOOGLE_OAUTH_CLIENT_SECRET")))
	})

	It("allows a development config without Google OAuth or domains", func() {
		cfg := env.Config{
			Environment:        "development",
			AdminDevLogin:      true,
			AdminSessionSecret: "dev-admin-session-secret-change-me",
		}
		Expect(cfg.Validate()).To(Succeed())
	})
})
