package env_test

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/luma/flightrecorder/env"
)

var _ = Describe("env/LoadConfig()", func() {
	It("maps REPORT_STORAGE_BACKEND onto Config", func() {
		previous, hadPrevious := os.LookupEnv("REPORT_STORAGE_BACKEND")
		Expect(os.Setenv("REPORT_STORAGE_BACKEND", "r2")).To(Succeed())
		DeferCleanup(func() {
			if hadPrevious {
				Expect(os.Setenv("REPORT_STORAGE_BACKEND", previous)).To(Succeed())
				return
			}
			Expect(os.Unsetenv("REPORT_STORAGE_BACKEND")).To(Succeed())
		})

		cfg, err := env.LoadConfig()

		Expect(err).ToNot(HaveOccurred())
		Expect(cfg.ReportStorageBackend).To(Equal("r2"))
	})

	It("defaults report storage to the local filesystem backend", func() {
		previous, hadPrevious := os.LookupEnv("REPORT_STORAGE_BACKEND")
		Expect(os.Unsetenv("REPORT_STORAGE_BACKEND")).To(Succeed())
		DeferCleanup(func() {
			if hadPrevious {
				Expect(os.Setenv("REPORT_STORAGE_BACKEND", previous)).To(Succeed())
				return
			}
			Expect(os.Unsetenv("REPORT_STORAGE_BACKEND")).To(Succeed())
		})

		cfg, err := env.LoadConfig()

		Expect(err).ToNot(HaveOccurred())
		Expect(cfg.ReportStorageBackend).To(Equal("local"))
	})
})
