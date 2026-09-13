package main_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// RefactorCLITestSuite drives "gotest refactor" through the built binary:
// toggle-focus edits the file it is given and nothing else, and the command
// refuses what it does not understand.
//
//nolint:lifecycle-pair // BeforeAll only wraps the shared binary, which its fixture removes
type RefactorCLITestSuite struct {
	CLI *gotestcli.BinarySharedFixture
	cli cliRunner
}

func (s *RefactorCLITestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type refactorCtx struct{ file string }

func (s *RefactorCLITestSuite) BeforeAll(t *gotest.T) {
	s.cli = newCLIRunner(s.CLI)
}

const focusFixture = `package sample

import "github.com/mvrahden/go-test/pkg/gotest"

type SampleTestSuite struct{}

func (s *SampleTestSuite) TestOne(t *gotest.T) {}
func (s *SampleTestSuite) TestTwo(t *gotest.T) {}
`

func (s *RefactorCLITestSuite) BeforeEach(t *gotest.T) *refactorCtx {
	file := filepath.Join(t.TempDir(), "sample_test.go")
	gotest.NoError(t, os.WriteFile(file, []byte(focusFixture), 0o600))
	return &refactorCtx{file: file}
}

func (s *RefactorCLITestSuite) read(t *gotest.T, ctx *refactorCtx) string {
	src, err := os.ReadFile(ctx.file)
	gotest.NoError(t, err)
	return string(src)
}

func (s *RefactorCLITestSuite) TestToggleFocus(t *gotest.T, ctx *refactorCtx) {
	t.When("the identifier names a suite", func(w *gotest.T) {
		out, code := s.cli.runExit(w, "refactor", "toggle-focus", ctx.file, "SampleTestSuite")
		w.It("adds the F_ prefix and reports it", func(it *gotest.T) {
			gotest.Equal(it, 0, code, out)
			gotest.Contains(it, out, "Toggled focus: SampleTestSuite")
			gotest.Contains(it, s.read(it, ctx), "F_SampleTestSuite")
		})
		w.It("removes the prefix again on the second toggle", func(it *gotest.T) {
			_, code := s.cli.runExit(it, "refactor", "toggle-focus", ctx.file, "F_SampleTestSuite")
			gotest.Equal(it, 0, code)
			gotest.NotContains(it, s.read(it, ctx), "F_")
		})
	})

	t.When("the identifier names a method", func(w *gotest.T) {
		_, code := s.cli.runExit(w, "refactor", "toggle-focus", ctx.file, "SampleTestSuite.TestTwo")
		w.It("prefixes that method and leaves its siblings alone", func(it *gotest.T) {
			gotest.Equal(it, 0, code)
			src := s.read(it, ctx)
			gotest.Contains(it, src, "F_TestTwo")
			gotest.Contains(it, src, ") TestOne(")
			gotest.NotContains(it, src, "F_SampleTestSuite")
		})
	})

	t.When("the identifier is unknown", func(w *gotest.T) {
		before := s.read(w, ctx)
		out, code := s.cli.runExit(w, "refactor", "toggle-focus", ctx.file, "NoSuchTestSuite")
		w.It("exits 1 and leaves the file untouched", func(it *gotest.T) {
			gotest.Equal(it, 1, code)
			gotest.Contains(it, out, "toggle-focus:")
			gotest.Equal(it, before, s.read(it, ctx))
		})
	})
}

func (s *RefactorCLITestSuite) TestUsage(t *gotest.T, _ *refactorCtx) {
	t.It("prints usage and exits 1 without a command", func(it *gotest.T) {
		out, code := s.cli.runExit(it, "refactor")
		gotest.Equal(it, 1, code)
		gotest.Contains(it, out, "usage: gotest refactor")
	})
	t.It("rejects an unknown command", func(it *gotest.T) {
		out, code := s.cli.runExit(it, "refactor", "rename-everything")
		gotest.Equal(it, 1, code)
		gotest.Contains(it, out, "unknown refactor command: rename-everything")
	})
	t.It("rejects toggle-focus without its two arguments", func(it *gotest.T) {
		out, code := s.cli.runExit(it, "refactor", "toggle-focus", "only-one")
		gotest.Equal(it, 1, code)
		gotest.Contains(it, out, "usage: gotest refactor toggle-focus")
	})
}
