package perf_test

import (
	"path/filepath"
	"regexp"
	"slices"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ActionsTestSuite holds a run to the build actions it repeats, counted in
// the tool calls GOFLAGS=-x prints on gotest's stderr.
type ActionsTestSuite struct {
	st staging
}

// Exclusive, as every perf suite: the CPU suite measures with nothing else
// of the run beside it.
func (s *ActionsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Exclusive: true, Timeout: 2 * time.Minute, SetupTimeout: 3 * time.Minute}
}

func (s *ActionsTestSuite) BeforeAll(t *gotest.T) { s.st.setUp(t) }

func (s *ActionsTestSuite) AfterAll(t *gotest.T) { s.st.tearDown(t) }

// The tool calls of go -x. go prefixes some with KEY=value words, and on
// Windows may quote the path.
var (
	compileRe = regexp.MustCompile(`(?m)^(?:\S+=\S+ )*"?\S*[/\\]compile(\.exe)?"? -o `)
	linkRe    = regexp.MustCompile(`(?m)^(?:\S+=\S+ )*"?\S*[/\\]link(\.exe)?"? -o `)
	vetRe     = regexp.MustCompile(`(?m)^(?:\S+=\S+ )*"?\S*[/\\]vet(\.exe)?"? `)
)

func (s *ActionsTestSuite) TestBuildActions(t *gotest.T) {
	env := append(slices.Clone(s.st.env), "GOFLAGS=-x")
	s.st.warm(t, s.st.m12, "./g/...")
	rerun, err := s.st.h.measure(t.Context(), s.st.m12, s.st.cli, env, "./g/...")
	gotest.NoError(t, err)
	t.When("nothing changed", func(w *gotest.T) {
		w.It("compiles, links and vets nothing", func(it *gotest.T) {
			gotest.Equal(it, 0, rerun.Exit, "stderr: %s", rerun.Stderr)
			gotest.Len(it, compileRe.FindAll(rerun.Stderr, -1), rerunCompiles)
			gotest.Len(it, linkRe.FindAll(rerun.Stderr, -1), rerunLinks)
			gotest.Len(it, vetRe.FindAll(rerun.Stderr, -1), rerunVets)
		})
	})
	gotest.NoError(t, edit(filepath.Join(s.st.m12, "g", "p07", "suites_test.go")))
	edited, err := s.st.h.measure(t.Context(), s.st.m12, s.st.cli, env, "./g/...")
	gotest.NoError(t, err)
	t.When("one test file changed", func(w *gotest.T) {
		w.It("compiles the package and its test main, links once, vets once", func(it *gotest.T) {
			gotest.Equal(it, 0, edited.Exit, "stderr: %s", edited.Stderr)
			gotest.Len(it, compileRe.FindAll(edited.Stderr, -1), editCompiles)
			gotest.Len(it, linkRe.FindAll(edited.Stderr, -1), editLinks)
			gotest.Len(it, vetRe.FindAll(edited.Stderr, -1), editVets)
		})
	})
}
