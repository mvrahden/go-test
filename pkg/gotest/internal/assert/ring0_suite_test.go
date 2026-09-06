// Ring 0: raw checks only, so a broken assertion engine cannot pass its own
// tests. No gotest.* assertion may appear in this package's tests. The kernel
// returns a failure message as a string, or "" on success, which is what makes
// plain comparisons sufficient here.
package assert_test //nolint:fail-guard

import (
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// fatalf records a failure and halts the test through testing.T alone.
func fatalf(t *gotest.T, format string, args ...any) {
	t.Errorf(format, args...)
	t.FailNow()
}

// mustFail checks that a kernel verdict is a failure message carrying every
// expected fragment.
func mustFail(t *gotest.T, verdict, what string, fragments ...string) {
	if verdict == "" {
		fatalf(t, "expected a failure message for %s", what)
	}
	for _, f := range fragments {
		if !strings.Contains(verdict, f) {
			t.Errorf("%s: message should contain %q, got: %q", what, f, verdict)
		}
	}
}

// mustPass checks that a kernel verdict is the empty success string.
func mustPass(t *gotest.T, verdict, what string) {
	if verdict != "" {
		t.Errorf("expected no failure for %s, got: %q", what, verdict)
	}
}

type ring0Ctx struct{}
