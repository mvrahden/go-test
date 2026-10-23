package callsite_test

import (
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
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
	t.It("names a file in another directory by its base name by default", func(it *gotest.T) {
		gotest.NoError(it, flag.Set("test.fullpath", "false"))
		gotest.Equal(it, "y.go:7", callsite.Declared("../other/y.go:7"))
	})
	t.It("resolves a file in another directory from the package directory under -test.fullpath", func(it *gotest.T) {
		gotest.NoError(it, flag.Set("test.fullpath", "true"))
		dir := gotest.Must(os.Getwd())
		gotest.Equal(it, filepath.ToSlash(filepath.Join(filepath.Dir(dir), "other", "y.go"))+":7", callsite.Declared("../other/y.go:7"))
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
		{"corp/tools.v2/helpers.Check", "corp/tools.v2", false},
		{"corp/tools%2ev2.F", "corp/tools.v2", false},
	} {
		t.It("judges "+c.fn+" knowing the main module is "+strconv.Quote(c.main), func(it *gotest.T) {
			gotest.Equal(it, c.std, callsite.ExportStdPathIn(c.fn, c.main))
		})
	}
	info := &debug.BuildInfo{
		Main: debug.Module{Path: "plainmod"},
		Deps: []*debug.Module{
			{Path: "plainhelpers", Version: "v0.0.0", Replace: &debug.Module{Path: "../plainhelpers"}},
			{Path: "plaindep", Version: "v1.0.0"},
			{Path: "example.com/dotted", Version: "v1.0.0"},
			{Path: "corp/tools.v2", Version: "v2.0.0"},
		},
	}
	for _, c := range []struct {
		fn  string
		std bool
	}{
		{"plainmod/helpers.expectAnswer", false},
		{"plainhelpers/check.ExpectAnswer", false},
		{"plaindep.F", false},
		{"example.com/dotted.F", false},
		{"corp/tools.v2/helpers.Check", false},
		{"corp/toolsx.F", true},
		{"plainhelpersx.F", true},
		{"fmt.Fprintln", true},
	} {
		t.It("judges "+c.fn+" knowing the modules the build required", func(it *gotest.T) {
			gotest.Equal(it, c.std, callsite.ExportStdPathInBuild(c.fn, info))
		})
	}
	t.It("judges by import path alone when the build recorded nothing", func(it *gotest.T) {
		gotest.True(it, callsite.ExportStdPathInBuild("plainmod/helpers.expectAnswer", nil))
	})
}
