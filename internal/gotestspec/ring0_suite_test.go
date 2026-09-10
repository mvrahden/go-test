// Ring 0: raw checks only, so a broken assertion engine cannot pass its own
// tests. No gotest.* assertion may appear in this file or its siblings that
// carry this header. Event parsing, tree building and stats decide which
// nodes count as failures, which is what every replay verdict rests on.
package gotestspec_test //nolint:fail-guard

import (
	"strings"

	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// fatalf records a failure and halts the test through testing.T alone.
func fatalf(t *gotest.T, format string, args ...any) {
	t.Errorf(format, args...)
	t.FailNow()
}

// mustEq reports a mismatch for a comparable value.
func mustEq[V comparable](t *gotest.T, what string, got, want V) {
	if got != want {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

// mustLen halts unless the slice has exactly n elements: what follows would
// index out of range otherwise.
func mustLen[E any](t *gotest.T, what string, got []E, n int) {
	if len(got) != n {
		fatalf(t, "%s: expected %d, got %d", what, n, len(got))
	}
}

// mustContain reports when s lacks sub.
func mustContain(t *gotest.T, s, sub, why string) {
	if !strings.Contains(s, sub) {
		t.Errorf("%s: expected %q in:\n%s", why, sub, s)
	}
}

// mustNotContain reports when s has sub.
func mustNotContain(t *gotest.T, s, sub, why string) {
	if strings.Contains(s, sub) {
		t.Errorf("%s: unexpected %q in:\n%s", why, sub, s)
	}
}

// treeOf parses a stream and builds its tree, halting on a parse error.
func treeOf(t *gotest.T, stream string) []*gotestspec.Package {
	events, err := gotestspec.ParseEvents(strings.NewReader(stream))
	if err != nil {
		fatalf(t, "ParseEvents: %v", err)
	}
	return gotestspec.BuildTree(events)
}
