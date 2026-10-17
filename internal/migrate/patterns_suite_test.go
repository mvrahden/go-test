package migrate_test

import (
	"github.com/mvrahden/go-test/internal/migrate"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// PatternsTestSuite covers how a package pattern splits into a root and a
// wildcard, and whether it is a path, under either path separator: the
// rule migrate reads every pattern by.
type PatternsTestSuite struct{}

func (s *PatternsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *PatternsTestSuite) TestCutWildcard(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc      string
		sep       rune
		root      string
		recursive bool
	}{
		{Desc: "./pkg/...", sep: '/', root: "./pkg", recursive: true},
		{Desc: "./...", sep: '/', root: ".", recursive: true},
		{Desc: "...", sep: '/', root: "", recursive: true},
		{Desc: "./pkg", sep: '/', root: "./pkg"},
		{Desc: `.\pkg\...`, sep: '\\', root: `.\pkg`, recursive: true},
		{Desc: `C:\x\...`, sep: '\\', root: `C:\x`, recursive: true},
		{Desc: `C:\x/...`, sep: '\\', root: `C:\x`, recursive: true},
		{Desc: `dir\...`, sep: '/', root: `dir\...`},
		{Desc: "example.com/m/...", sep: '/', root: "example.com/m", recursive: true},
	}) {
		root, recursive := migrate.CutWildcard(tc.Desc, tc.sep)
		gotest.Equal(sub, tc.root, root)
		gotest.Equal(sub, tc.recursive, recursive)
	}
}

func (s *PatternsTestSuite) TestIsPathPattern(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc string
		sep  rune
		path bool
	}{
		{Desc: ".", sep: '/', path: true},
		{Desc: "./pkg", sep: '/', path: true},
		{Desc: "../pkg", sep: '/', path: true},
		{Desc: "/abs/pkg", sep: '/', path: true},
		{Desc: "example.com/m/pkg", sep: '/'},
		{Desc: `.\pkg`, sep: '\\', path: true},
		{Desc: `..\pkg`, sep: '\\', path: true},
		{Desc: `C:\x`, sep: '\\', path: true},
		{Desc: `C:/x`, sep: '\\', path: true},
		{Desc: `\\server\share`, sep: '\\', path: true},
		{Desc: `.\pkg`, sep: '/'},
		{Desc: "example.com/m/pkg", sep: '\\'},
	}) {
		gotest.Equal(sub, tc.path, migrate.IsPathPattern(tc.Desc, tc.sep))
	}
}
