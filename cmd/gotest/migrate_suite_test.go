package main_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// MigrateCLITestSuite drives "gotest migrate" through the built binary: its
// exit code is the answer to "is anything left to do by hand", and
// --dry-run shows the edit without making it.
//
//nolint:lifecycle-pair // BeforeAll only wraps the shared binary, which its fixture removes
type MigrateCLITestSuite struct {
	CLI *gotestcli.BinarySharedFixture
	cli cliRunner
}

func (s *MigrateCLITestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *MigrateCLITestSuite) BeforeAll(t *gotest.T) {
	s.cli = newCLIRunner(s.CLI)
}

type migrateCtx struct {
	dir  string
	path string
}

const migrateCleanSrc = `package sample

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type CleanSuite struct {
	suite.Suite
}

func (s *CleanSuite) TestOK() {
	s.Require().Equal(1, 1)
}

func TestCleanSuite(t *testing.T) {
	suite.Run(t, new(CleanSuite))
}
`

const migrateMarkedSrc = `package sample

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type MarkedSuite struct {
	suite.Suite
}

func (s *MarkedSuite) TestOK() {
	var target error
	s.Require().ErrorAs(nil, &target)
}

func TestMarkedSuite(t *testing.T) {
	suite.Run(t, new(MarkedSuite))
}
`

func (s *MigrateCLITestSuite) BeforeEach(t *gotest.T) *migrateCtx {
	dir := t.TempDir()
	return &migrateCtx{dir: dir, path: filepath.Join(dir, "sample_test.go")}
}

func (s *MigrateCLITestSuite) write(t *gotest.T, ctx *migrateCtx, src string) {
	gotest.NoError(t, os.WriteFile(ctx.path, []byte(src), 0o600))
}

func (s *MigrateCLITestSuite) read(t *gotest.T, ctx *migrateCtx) string {
	got, err := os.ReadFile(ctx.path)
	gotest.NoError(t, err)
	return string(got)
}

func (s *MigrateCLITestSuite) TestExitCode(t *gotest.T, ctx *migrateCtx) {
	t.When("everything converted", func(w *gotest.T) {
		s.write(w, ctx, migrateCleanSrc)
		out, code := runGotestIn(w, s.cli.binary, ctx.dir, nil, "migrate", ".")
		w.It("exits 0", func(it *gotest.T) {
			gotest.Equal(it, 0, code, out)
			gotest.Contains(it, out, "CleanSuite → CleanTestSuite")
		})
	})

	t.When("a marker was left", func(w *gotest.T) {
		s.write(w, ctx, migrateMarkedSrc)
		out, code := runGotestIn(w, s.cli.binary, ctx.dir, nil, "migrate", ".")
		w.It("exits 1 and counts the markers", func(it *gotest.T) {
			gotest.Equal(it, 1, code, out)
			gotest.Contains(it, out, "1 TODO(gotest-migrate) marker")
			gotest.Contains(it, s.read(it, ctx), "TODO(gotest-migrate)")
		})
	})
}

func (s *MigrateCLITestSuite) TestDryRun(t *gotest.T, ctx *migrateCtx) {
	s.write(t, ctx, migrateMarkedSrc)
	out, code := runGotestIn(t, s.cli.binary, ctx.dir, nil, "migrate", "--dry-run", ".")

	t.It("prints the diff and the markers it would leave", func(it *gotest.T) {
		gotest.Contains(it, out, "--- a/sample_test.go")
		gotest.Contains(it, out, "+++ b/sample_test.go")
		gotest.Contains(it, out, "+type MarkedTestSuite struct {")
		gotest.Contains(it, out, "+\t// TODO(gotest-migrate): unmapped assertion ErrorAs")
	})

	t.It("writes nothing", func(it *gotest.T) {
		gotest.Equal(it, migrateMarkedSrc, s.read(it, ctx))
	})

	t.It("still exits 1 for the markers", func(it *gotest.T) {
		gotest.Equal(it, 1, code, out)
	})
}
