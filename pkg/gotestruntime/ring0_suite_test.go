// Ring 0: raw checks only, so a broken assertion engine cannot pass its own
// tests. No gotest.* assertion may appear in this file or its siblings that
// carry this header. The runtime decides lifecycle order, teardown, retries
// and exit codes, which every suite's verdict rests on.
package gotestruntime_test //nolint:fail-guard

import (
	"cmp"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

func fatalf(t *gotest.T, format string, args ...any) {
	t.Errorf(format, args...)
	t.FailNow()
}

// note renders the optional trailing message of a check.
func note(msgAndArgs []any) string {
	if len(msgAndArgs) == 0 {
		return ""
	}
	if format, ok := msgAndArgs[0].(string); ok && len(msgAndArgs) > 1 {
		return ": " + fmt.Sprintf(format, msgAndArgs[1:]...)
	}
	return ": " + fmt.Sprint(msgAndArgs...)
}

func mustEqual(t *gotest.T, expected, actual any, msgAndArgs ...any) {
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("not equal%s\n  expected: %#v\n  actual:   %#v", note(msgAndArgs), expected, actual)
	}
}

func mustLess[V cmp.Ordered](t *gotest.T, a, b V, msgAndArgs ...any) {
	if !(a < b) {
		t.Errorf("%v is not less than %v%s", a, b, note(msgAndArgs))
	}
}

func mustGreater[V cmp.Ordered](t *gotest.T, a, b V, msgAndArgs ...any) {
	if !(a > b) {
		t.Errorf("%v is not greater than %v%s", a, b, note(msgAndArgs))
	}
}

func mustGreaterOrEqual[V cmp.Ordered](t *gotest.T, a, b V, msgAndArgs ...any) {
	if !(a >= b) {
		t.Errorf("%v is not greater than or equal to %v%s", a, b, note(msgAndArgs))
	}
}

// holds reports whether container (a string or a []string) contains elem.
func holds(container, elem any) bool {
	switch c := container.(type) {
	case string:
		return strings.Contains(c, elem.(string))
	case []string:
		for _, s := range c {
			if s == elem {
				return true
			}
		}
		return false
	default:
		panic("holds: unsupported container")
	}
}

func mustContain(t *gotest.T, container, elem any, msgAndArgs ...any) {
	if !holds(container, elem) {
		t.Errorf("%#v does not contain %#v%s", container, elem, note(msgAndArgs))
	}
}

func mustNotContain(t *gotest.T, container, elem any, msgAndArgs ...any) {
	if holds(container, elem) {
		t.Errorf("%#v contains %#v%s", container, elem, note(msgAndArgs))
	}
}

func mustErrorContain(t *gotest.T, err error, sub string, msgAndArgs ...any) {
	if err == nil {
		fatalf(t, "expected an error containing %q, got nil%s", sub, note(msgAndArgs))
	}
	if !strings.Contains(err.Error(), sub) {
		t.Errorf("error %q does not contain %q%s", err.Error(), sub, note(msgAndArgs))
	}
}

func mustErrorIs(t *gotest.T, err, target error, msgAndArgs ...any) {
	if !errors.Is(err, target) {
		t.Errorf("error %v is not %v%s", err, target, note(msgAndArgs))
	}
}

func mustNoError(t *gotest.T, err error, msgAndArgs ...any) {
	if err != nil {
		fatalf(t, "unexpected error: %v%s", err, note(msgAndArgs))
	}
}

func mustTrue(t *gotest.T, v bool, msgAndArgs ...any) {
	if !v {
		t.Errorf("expected true%s", note(msgAndArgs))
	}
}

func mustFalse(t *gotest.T, v bool, msgAndArgs ...any) {
	if v {
		t.Errorf("expected false%s", note(msgAndArgs))
	}
}

func mustLen(t *gotest.T, object any, n int, msgAndArgs ...any) {
	if got := reflect.ValueOf(object).Len(); got != n {
		fatalf(t, "length %d, want %d%s", got, n, note(msgAndArgs))
	}
}
