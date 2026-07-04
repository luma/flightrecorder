package auth_test

import (
	"context"
	"errors"
	"log/slog"

	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/luma/flightrecorder/api/auth"
	"github.com/luma/flightrecorder/services"
)

type fakeAuthService struct {
	projectID uuid.UUID
	err       error
	tokenHash string
}

func (s *fakeAuthService) ValidateAccessToken(_ context.Context, tokenHash string) (uuid.UUID, error) {
	s.tokenHash = tokenHash
	if s.err != nil {
		return uuid.Nil, s.err
	}
	return s.projectID, nil
}

var _ = Describe("RequireAuth", func() {
	It("rejects requests without a bearer token", func() {
		c := ut.CreateUtRequestContext("POST", "/v1/events", nil)
		middleware := auth.RequireAuth(&fakeAuthService{}, slog.Default())

		middleware(context.Background(), c)

		Expect(c.Response.StatusCode()).To(Equal(consts.StatusUnauthorized))
		Expect(c.IsAborted()).To(BeTrue())
	})

	It("hashes bearer tokens before validation", func() {
		token := services.TelemetryTokenPrefix + "test-token"
		service := &fakeAuthService{projectID: uuid.New()}
		c := ut.CreateUtRequestContext("POST", "/v1/events", nil, ut.Header{
			Key:   "Authorization",
			Value: "Bearer " + token,
		})
		middleware := auth.RequireAuth(service, slog.Default())

		middleware(context.Background(), c)

		Expect(service.tokenHash).To(Equal(auth.HashToken(token)))
	})

	It("rejects invalid bearer tokens", func() {
		c := ut.CreateUtRequestContext("POST", "/v1/events", nil, ut.Header{
			Key:   "Authorization",
			Value: "Bearer " + services.TelemetryTokenPrefix + "bad-token",
		})
		middleware := auth.RequireAuth(&fakeAuthService{err: errors.New("nope")}, slog.Default())

		middleware(context.Background(), c)

		Expect(c.Response.StatusCode()).To(Equal(consts.StatusUnauthorized))
		Expect(c.IsAborted()).To(BeTrue())
	})

	It("rejects agent tokens on ingest routes", func() {
		service := &fakeAuthService{projectID: uuid.New()}
		c := ut.CreateUtRequestContext("POST", "/v1/events", nil, ut.Header{
			Key:   "Authorization",
			Value: "Bearer " + services.AgentTokenPrefix + "agent-token",
		})
		middleware := auth.RequireAuth(service, slog.Default())

		middleware(context.Background(), c)

		Expect(service.tokenHash).To(BeEmpty())
		Expect(c.Response.StatusCode()).To(Equal(consts.StatusUnauthorized))
		Expect(c.IsAborted()).To(BeTrue())
	})
})
