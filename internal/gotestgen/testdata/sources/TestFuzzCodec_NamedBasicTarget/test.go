package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

// Level is fuzzed directly — no enclosing struct — so its literal function
// must be built by literalHelper as a top-level entry point (the one case
// literalExpr itself never routes through literalHelper for, since a basic
// field is always inlined into its container's literal function instead).
type Level int

type NamedBasicTestSuite struct{}

func (s *NamedBasicTestSuite) FuzzLevel(f *gotest.F) {
	gotest.Fuzz(f, func(t *gotest.T, l Level) { _ = l })
}
