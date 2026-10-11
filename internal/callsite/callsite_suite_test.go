package callsite_test

import (
	"runtime"

	"github.com/mvrahden/go-test/internal/callsite"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CallsiteTestSuite covers naming locations as go test does.
type CallsiteTestSuite struct{}

func (s *CallsiteTestSuite) TestIsStd(t *gotest.T) {
	t.It("knows a standard library frame by its file", func(it *gotest.T) {
		var pcs [1]uintptr
		runtime.Callers(0, pcs[:]) // runtime.Callers itself
		frame, _ := runtime.CallersFrames(pcs[:]).Next()
		gotest.True(it, callsite.IsStd(frame.Function, frame.File), frame.File)
	})
	t.It("knows the user's frame", func(it *gotest.T) {
		var pcs [1]uintptr
		runtime.Callers(1, pcs[:])
		frame, _ := runtime.CallersFrames(pcs[:]).Next()
		gotest.False(it, callsite.IsStd(frame.Function, frame.File), frame.File)
	})
	for _, c := range []struct {
		fn  string
		std bool
	}{
		{"fmt.Fprintln({0x1, 0x2})", true},
		{"net/http.(*Server).Serve(...)", true},
		{"reflect.Value.call({0x1, 0x2, 0x3})", true},
		{"example.com/m.F", false},
		{"github.com/x/y.(*T).M(...)", false},
		{"main.main()", false},
	} {
		t.It("judges "+c.fn+" by its import path under -trimpath", func(it *gotest.T) {
			gotest.Equal(it, c.std, callsite.ExportStdPath(c.fn))
		})
	}
}
