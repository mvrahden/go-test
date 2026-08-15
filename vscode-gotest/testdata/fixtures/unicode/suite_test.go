package unicode

import "github.com/mvrahden/go-test/pkg/gotest"

// Non-ASCII in behavior names and in failure output: multibyte characters must
// survive the JSON envelope, the spec tree, and HTML rendering without being
// mangled or truncated mid-rune.
type UnicodeTestSuite struct{}

func (s *UnicodeTestSuite) TestInternational(t *gotest.T) {
	t.When("behavior names carry non-ASCII text", func(t *gotest.T) {
		t.It("handles accented Latin — café naïve", func(t *gotest.T) {
			gotest.Equal(t, "café", "café")
		})
		t.It("handles CJK — 日本語のテスト", func(t *gotest.T) {
			gotest.Equal(t, "日本語", "日本語")
		})
		t.It("handles emoji and RTL — 🧪 مرحبا", func(t *gotest.T) {
			gotest.Equal(t, "🧪", "🧪")
		})
		t.It("reports a failure containing non-ASCII", func(t *gotest.T) {
			gotest.Equal(t, "期待値", "実際の値 🚫")
		})
	})
}
