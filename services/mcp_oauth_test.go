package services

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("MCP OAuth helpers", func() {
	It("defaults omitted scopes to read and write", func() {
		scopes, err := normalizeScopes("")

		Expect(err).ToNot(HaveOccurred())
		Expect(scopes).To(Equal([]string{MCPReadScope, MCPWriteScope}))
	})

	It("preserves valid requested scopes", func() {
		scopes, err := normalizeScopes(MCPReadScope)

		Expect(err).ToNot(HaveOccurred())
		Expect(scopes).To(Equal([]string{MCPReadScope}))
	})

	It("rejects unsupported scopes", func() {
		_, err := normalizeScopes("mcp:admin")

		Expect(err).To(MatchError(ContainSubstring("invalid_scope")))
	})
})
