package services

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("admin projects", func() {
	It("normalizes create project requests into upsert params", func() {
		params, err := createProjectParams(CreateProjectRequest{
			ProjectID:   "example_game",
			DisplayName: "Example Game",
		})

		Expect(err).ToNot(HaveOccurred())
		Expect(params.ProjectKey).To(Equal("example_game"))
		Expect(params.DisplayName).To(Equal("Example Game"))
		Expect(params.ValidationMode).To(Equal("warn"))
		Expect(json.Valid(params.IngestConfig)).To(BeTrue())
		Expect(json.Valid(params.QueryFields)).To(BeTrue())
	})

	It("rejects unsafe project keys", func() {
		_, err := createProjectParams(CreateProjectRequest{
			ProjectID:   "Example Game",
			DisplayName: "Example Game",
		})

		Expect(err).To(MatchError(ContainSubstring("project_id must use lowercase")))
	})

	It("does not substitute a default project key", func() {
		Expect(requiredProjectKey("")).To(BeEmpty())
		Expect(requiredProjectKey(" example ")).To(Equal("example"))
	})

	It("rejects unsupported query field types", func() {
		_, err := createProjectParams(CreateProjectRequest{
			ProjectID:   "example",
			DisplayName: "Example",
			QueryFields: json.RawMessage(`[{"key":"ship","source":"dimensions.ship","type":"object"}]`),
		})

		Expect(err).To(MatchError(ContainSubstring("query_fields type must be string, number, or bool")))
	})
})
