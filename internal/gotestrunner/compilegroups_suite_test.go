package gotestrunner_test

import (
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CompileGroupsTestSuite: test binaries are named as go test -c names them,
// and packages that share a name are built apart.
type CompileGroupsTestSuite struct{}

func (s *CompileGroupsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *CompileGroupsTestSuite) TestBinaryNames(t *gotest.T) {
	type row struct{ Desc, Path, GOOS, Want string }
	for t, r := range gotest.Each(t, []row{
		{"the last element", "example.com/m/a/util", "linux", "util.test"},
		{"the element before a version", "example.com/m/v2", "linux", "m.test"},
		{"v1 is not a version element", "example.com/m/v1", "linux", "v1.test"},
		{"v0 is not a version element", "example.com/m/v0", "linux", "v0.test"},
		{"two digits", "example.com/m/v10", "linux", "m.test"},
		{"letters after the digits", "example.com/m/v2a", "linux", "v2a.test"},
		{"a path of one element", "v2", "linux", "v2.test"},
		{"windows", "example.com/m/util", "windows", "util.test.exe"},
	}) {
		gotest.Equal(t, r.Want, gotestrunner.ExportTestBinaryName(r.Path, r.GOOS))
	}
}

func (s *CompileGroupsTestSuite) TestGroups(t *gotest.T) {
	t.It("puts packages with distinct names in one group, in order", func(it *gotest.T) {
		gotest.Equal(it, [][]string{{"m/a", "m/b", "m/c"}}, gotestrunner.ExportCompileGroups([]string{"m/a", "m/b", "m/c"}, "linux"))
	})
	t.It("deals a repeated name into the first group without it", func(it *gotest.T) {
		got := gotestrunner.ExportCompileGroups([]string{"m/a/util", "m/x", "m/b/util", "m/c/util", "m/y"}, "linux")
		gotest.Equal(it, [][]string{{"m/a/util", "m/x", "m/y"}, {"m/b/util"}, {"m/c/util"}}, got)
	})
	t.It("holds names apart that differ only by case", func(it *gotest.T) {
		gotest.Len(it, gotestrunner.ExportCompileGroups([]string{"m/Util", "m/x/util"}, "linux"), 2)
	})
	t.It("has no group for no package", func(it *gotest.T) {
		gotest.Empty(it, gotestrunner.ExportCompileGroups(nil, "linux"))
	})
}
