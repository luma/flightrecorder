package requestid_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/luma/flightrecorder/api/requestid"
)

func TestRequestID(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RequestID Suite")
}

var _ = Describe("RequestID", func() {
	var logger *slog.Logger

	BeforeEach(func() {
		logger = slog.Default()
	})

	Describe("Middleware", func() {
		It("generates a request ID when none provided", func() {
			middleware := requestid.Middleware(logger)

			c := ut.CreateUtRequestContext("GET", "/test", nil)

			middleware(context.Background(), c)

			responseID := string(c.Response.Header.Peek(requestid.Header))
			Expect(responseID).ToNot(BeEmpty())
			Expect(responseID).To(HaveLen(36))
		})

		It("uses provided request ID from header", func() {
			middleware := requestid.Middleware(logger)

			c := ut.CreateUtRequestContext("GET", "/test", nil,
				ut.Header{Key: requestid.Header, Value: "custom-request-id-123"})

			middleware(context.Background(), c)

			responseID := string(c.Response.Header.Peek(requestid.Header))
			Expect(responseID).To(Equal("custom-request-id-123"))
		})
	})

	Describe("Context functions", func() {
		It("stores and retrieves request ID from context", func() {
			ctx := requestid.WithRequestID(context.Background(), "test-id-456")
			Expect(requestid.FromContext(ctx)).To(Equal("test-id-456"))
		})

		It("returns empty string when no request ID in context", func() {
			Expect(requestid.FromContext(context.Background())).To(BeEmpty())
		})

		It("stores and retrieves logger from context", func() {
			testLogger := slog.Default()
			ctx := requestid.WithLogger(context.Background(), testLogger)
			Expect(requestid.LoggerFromContext(ctx)).To(Equal(testLogger))
		})

		It("returns nil when no logger in context", func() {
			Expect(requestid.LoggerFromContext(context.Background())).To(BeNil())
		})

		It("panics when MustLoggerFromContext called without logger", func() {
			Expect(func() {
				requestid.MustLoggerFromContext(context.Background())
			}).To(Panic())
		})
	})
})
