package callsite_test

import (
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/mvrahden/go-test/internal/callsite"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CallsiteTestSuite covers naming locations as go test does.
// Sequential: sets the process-wide -test.fullpath flag.
type CallsiteTestSuite struct{}

func (s *CallsiteTestSuite) AfterEach(t *gotest.T) {
	gotest.NoError(t, flag.Set("test.fullpath", "false"))
}

func (s *CallsiteTestSuite) TestFormat(t *gotest.T) {
	t.It("names the base name by default", func(it *gotest.T) {
		gotest.Equal(it, "x_test.go:7", callsite.Format("/src/pkg/x_test.go", 7))
	})
	t.It("names the full path under -test.fullpath", func(it *gotest.T) {
		gotest.NoError(it, flag.Set("test.fullpath", "true"))
		gotest.Equal(it, "/src/pkg/x_test.go:7", callsite.Format("/src/pkg/x_test.go", 7))
	})
}

func (s *CallsiteTestSuite) TestDeclared(t *gotest.T) {
	t.It("keeps the recorded base name by default", func(it *gotest.T) {
		gotest.Equal(it, "x_test.go:7", callsite.Declared("x_test.go:7"))
	})
	t.It("joins the package directory under -test.fullpath", func(it *gotest.T) {
		gotest.NoError(it, flag.Set("test.fullpath", "true"))
		dir := gotest.Must(os.Getwd())
		gotest.Equal(it, filepath.ToSlash(filepath.Join(dir, "x_test.go"))+":7", callsite.Declared("x_test.go:7"))
	})
	t.It("leaves no location empty", func(it *gotest.T) {
		gotest.NoError(it, flag.Set("test.fullpath", "true"))
		gotest.Empty(it, callsite.Declared(""))
	})
}

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
	for _, c := range []struct {
		fn   string
		main string
		std  bool
	}{
		{"plainmod/helpers.expectAnswer", "plainmod", false},
		{"plainmod.F", "plainmod", false},
		{"plainmodule/x.F", "plainmod", true},
		{"fmt.Fprintln", "plainmod", true},
		{"plainmod/helpers.expectAnswer", "", true},
	} {
		t.It("judges "+c.fn+" knowing the main module is "+strconv.Quote(c.main), func(it *gotest.T) {
			gotest.Equal(it, c.std, callsite.ExportStdPathIn(c.fn, c.main))
		})
	}
}
