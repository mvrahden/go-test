package gotestruntime

import (
	"io"
	"testing"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ExportDefaultDeadlineOut sends the default-deadline line to w until the
// returned call restores where it goes.
func ExportDefaultDeadlineOut(w io.Writer) (restore func()) {
	prev := defaultDeadlineOut
	defaultDeadlineOut = func(testing.TB) io.Writer { return w }
	return func() { defaultDeadlineOut = prev }
}

// ExportOpenMethod opens a test method as RunMethods does, returning its T
// and the call that ends it.
func ExportOpenMethod(it *testing.T, timeout, budget time.Duration) (*gotest.T, func()) {
	m, end := openMethod(it, timeout, budget)
	return m.t, end
}

// ExportNoteDefaultDeadline is noteDefaultDeadline.
var ExportNoteDefaultDeadline = noteDefaultDeadline
