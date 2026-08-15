package gotestast_test

import (
	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SubtestNameTestSuite pins the one thing that must match byte for byte: the
// name go test gives a subtest. A statically derived name that differs from
// the observed one would put the same behavior in the tree twice.
type SubtestNameTestSuite struct{}

func (s *SubtestNameTestSuite) TestRewriting(t *gotest.T) {
	t.When("a description is turned into a subtest name", func(w *gotest.T) {
		for sub, tc := range gotest.Each(w, []struct {
			Desc string
			in   string
			want string
		}{
			{Desc: "spaces become underscores", in: "summing a basket", want: "summing_a_basket"},
			{Desc: "tabs and newlines are spaces too", in: "a\tb\nc", want: "a_b_c"},
			{Desc: "runs of spaces each map to one underscore", in: "a  b", want: "a__b"},
			{Desc: "slashes survive, since go test keeps them", in: "https:// URI", want: "https://_URI"},
			{Desc: "non-ASCII printable text is preserved", in: "café 日本語 🧪", want: "café_日本語_🧪"},
			{Desc: "leading and trailing spaces still map", in: " x ", want: "_x_"},
			{Desc: "an empty description stays empty", in: "", want: ""},
		}) {
			gotest.Equal(sub, tc.want, gotestast.SubtestName(tc.in))
		}
	})
}
