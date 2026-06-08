package services

import (
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("screenshot stores", func() {
	It("stores local report screenshots under the shared object key layout", func() {
		root := GinkgoT().TempDir()
		store := LocalScreenshotStore{RootDir: root}
		eventTime := time.Date(2026, 6, 6, 11, 42, 0, 0, time.UTC)

		key, err := store.StorePNG(context.Background(), "sursidus", "report-001", eventTime, []byte("png"))

		Expect(err).ToNot(HaveOccurred())
		Expect(key).To(Equal("bug-reports/sursidus/2026/06/06/report-001.png"))
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(key)))
		Expect(err).ToNot(HaveOccurred())
		Expect(data).To(Equal([]byte("png")))
	})

	It("rejects local storage without a root directory", func() {
		_, err := LocalScreenshotStore{}.StorePNG(context.Background(), "sursidus", "report-001", time.Now(), []byte("png"))

		Expect(err).To(MatchError("root directory is required"))
	})

	It("sanitizes object key components before writing local screenshots", func() {
		root := GinkgoT().TempDir()
		store := LocalScreenshotStore{RootDir: root}
		eventTime := time.Date(2026, 6, 6, 11, 42, 0, 0, time.UTC)

		key, err := store.StorePNG(context.Background(), "../sursidus", "../../report-001", eventTime, []byte("png"))

		Expect(err).ToNot(HaveOccurred())
		Expect(key).To(Equal("bug-reports/.._sursidus/2026/06/06/.._.._report-001.png"))
		Expect(filepath.IsLocal(key)).To(BeTrue())
		Expect(key).ToNot(ContainSubstring("/../"))
		_, err = os.Stat(filepath.Join(root, filepath.FromSlash(key)))
		Expect(err).ToNot(HaveOccurred())
	})
})
