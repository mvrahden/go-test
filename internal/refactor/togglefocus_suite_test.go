package refactor_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/refactor"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ToggleFocusTestSuite covers the F_ prefix toggle on suites and methods; the
// edit must be exact text, reversible, and refuse unknown targets.
type ToggleFocusTestSuite struct{}

func (s *ToggleFocusTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type toggleCtx struct{ path string }

func (s *ToggleFocusTestSuite) BeforeEach(t *gotest.T) *toggleCtx {
	return &toggleCtx{path: filepath.Join(t.TempDir(), "foo_test.go")}
}

func (s *ToggleFocusTestSuite) write(t *gotest.T, ctx *toggleCtx, src string) {
	gotest.NoError(t, os.WriteFile(ctx.path, []byte(src), 0o600))
}

func (s *ToggleFocusTestSuite) read(t *gotest.T, ctx *toggleCtx) string {
	got, err := os.ReadFile(ctx.path)
	gotest.NoError(t, err)
	return string(got)
}

const twoMethodSuite = `package example

type FooTestSuite struct{}

func (s *FooTestSuite) TestBar() {}
func (s *FooTestSuite) TestBaz() {}
`

func (s *ToggleFocusTestSuite) TestToggleFocus_Suite(t *gotest.T, ctx *toggleCtx) {
	s.write(t, ctx, twoMethodSuite)

	gotest.NoError(t, refactor.ToggleFocus(ctx.path, "FooTestSuite"))
	gotest.Equal(t, `package example

type F_FooTestSuite struct{}

func (s *F_FooTestSuite) TestBar() {}
func (s *F_FooTestSuite) TestBaz() {}
`, s.read(t, ctx))

	gotest.NoError(t, refactor.ToggleFocus(ctx.path, "F_FooTestSuite"))
	gotest.Equal(t, twoMethodSuite, s.read(t, ctx))
}

func (s *ToggleFocusTestSuite) TestToggleFocus_Method(t *gotest.T, ctx *toggleCtx) {
	s.write(t, ctx, twoMethodSuite)

	gotest.NoError(t, refactor.ToggleFocus(ctx.path, "FooTestSuite.TestBar"))
	gotest.Equal(t, `package example

type FooTestSuite struct{}

func (s *FooTestSuite) F_TestBar() {}
func (s *FooTestSuite) TestBaz() {}
`, s.read(t, ctx))

	gotest.NoError(t, refactor.ToggleFocus(ctx.path, "FooTestSuite.F_TestBar"))
	gotest.Equal(t, twoMethodSuite, s.read(t, ctx))
}

func (s *ToggleFocusTestSuite) TestToggleFocus_ValueReceiver(t *gotest.T, ctx *toggleCtx) {
	s.write(t, ctx, `package example

type BarTestSuite struct{}

func (s BarTestSuite) TestOne() {}
`)

	gotest.NoError(t, refactor.ToggleFocus(ctx.path, "BarTestSuite"))
	gotest.Equal(t, `package example

type F_BarTestSuite struct{}

func (s F_BarTestSuite) TestOne() {}
`, s.read(t, ctx))
}

func (s *ToggleFocusTestSuite) TestToggleFocus_NotFound(t *gotest.T, ctx *toggleCtx) {
	s.write(t, ctx, `package example

type FooTestSuite struct{}
`)

	gotest.Error(t, refactor.ToggleFocus(ctx.path, "NonExistent"))
	gotest.Error(t, refactor.ToggleFocus(ctx.path, "FooTestSuite.NonExistent"))
}
