package lintpollnames

import (
	"bytes"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// Equal is this package's own, on plain values.
func Equal(n, want int) bool { return n == want }

// NoError is another library's assertion: it takes the test.
func NoError(t *testing.T, err error) {}

func TestNames(t *testing.T) { //nolint:stdlib-test
	out, a, b, xs, n := "ready", []byte("a"), []byte("a"), []int{1}, 0
	gotest.Eventually(t, time.Second, time.Millisecond, func(poll *gotest.R) {
		_ = strings.Contains(out, "ready")
		_ = bytes.Equal(a, b)
		_ = slices.Contains(xs, 1)
		_ = Equal(n, 0)
		NoError(t, nil)
	})
}
