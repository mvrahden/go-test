package conventions_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ClauseListingTestSuite: the clause listing the drill reads is the
// scenarios of specs/, one "<WHEN> → <THEN>" line per THEN and AND, the WHEN
// spelled as gotest spec renders a When label.
type ClauseListingTestSuite struct {
	root   string
	binDir string
	bin    string
}

func (s *ClauseListingTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *ClauseListingTestSuite) BeforeAll(t *gotest.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	gotest.NoError(t, err)
	s.root = root
	s.binDir, err = os.MkdirTemp("", "clauses")
	gotest.NoError(t, err)
	s.bin = filepath.Join(s.binDir, "clauses")
	if runtime.GOOS == "windows" {
		s.bin += ".exe"
	}
	cmd := exec.CommandContext(t.Context(), "go", "build", "-o", s.bin, "./internal/speccatalog/clauses") //nolint:gosec // G204: the repository's own command
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	gotest.NoError(t, err, string(out))
}

func (s *ClauseListingTestSuite) AfterAll(t *gotest.T) {
	if s.binDir != "" {
		gotest.NoError(t, os.RemoveAll(s.binDir))
	}
}

// listing runs the clause listing with dir as the repository root.
func (s *ClauseListingTestSuite) listing(t *gotest.T, dir string, args ...string) []string {
	cmd := exec.CommandContext(t.Context(), s.bin, args...) //nolint:gosec // G204: the binary built in BeforeAll
	cmd.Dir = dir
	out, err := cmd.Output()
	gotest.NoError(t, err)
	return strings.Split(strings.TrimRight(string(out), "\n"), "\n")
}

const listingSpec = `# Run Specification

## Purpose
How a run reads the conditions it is given.

## Requirements

### Requirement: Stopping
A run SHALL tear down when it stops.

#### Scenario: Stopped
- **WHEN** When the run stops
- **THEN** tears down every fixture
- **AND** exits 1

#### Scenario: Cached
- **WHEN** with an empty cache
- **THEN** builds every package

#### Scenario: Plain
- **WHEN** a flag is unknown
- **THEN** exits 2
`

const listingPatch = "Clause: when the run stops → exits 1\nClause: with an empty cache → builds every package\nExpect: x\n--- a/x.go\n+++ b/x.go\n"

// fixtureRepo writes a repository holding one spec and one mutant.
func fixtureRepo(t *gotest.T) string {
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":                          "module github.com/mvrahden/go-test\n\ngo 1.26.0\n",
		"specs/run/spec.md":               listingSpec,
		"tests/drill/mutants/stop.patch":  listingPatch,
		"tests/drill/unmutated-allow.txt": "2\n",
	}
	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		gotest.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		gotest.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	return dir
}

func (s *ClauseListingTestSuite) TestListingIsTheScenarios(t *gotest.T) {
	t.When("the clause listing reads a spec whose WHENs open with \"When\", a connective and neither", func(w *gotest.T) {
		got := s.listing(w, fixtureRepo(w))
		w.It("prints each WHEN and THEN pair once, in spec order", func(it *gotest.T) {
			gotest.Equal(it, []string{
				"when the run stops → tears down every fixture",
				"when the run stops → exits 1",
				"with an empty cache → builds every package",
				"when a flag is unknown → exits 2",
			}, got)
		})
	})
	t.When("the clause listing reads specs/", func(w *gotest.T) {
		got := s.listing(w, s.root)
		w.It("holds clauses the specs state", func(it *gotest.T) {
			gotest.Subset(it, got, []string{
				"when the version subcommand runs → exits 0 and prints the version on stdout",
				"when stdout is a pipe → prints no color codes, even with --no-color=false",
				"when --gate=0 is given → compares without a gate, whatever -count says",
			})
		})
	})
}

func (s *ClauseListingTestSuite) TestUnmutatedListingIsTheBudgetReading(t *gotest.T) {
	t.When("the unmutated listing reads a spec and a mutant naming two of its clauses", func(w *gotest.T) {
		got := s.listing(w, fixtureRepo(w), "--unmutated")
		w.It("prints the clauses no Clause line names, sorted", func(it *gotest.T) {
			gotest.Equal(it, []string{
				"when a flag is unknown → exits 2",
				"when the run stops → tears down every fixture",
			}, got)
		})
	})
	t.When("the unmutated listing reads specs/ and the drill's mutants", func(w *gotest.T) {
		got := s.listing(w, s.root, "--unmutated")
		data, err := os.ReadFile(filepath.Join(s.root, "tests", "drill", "unmutated-allow.txt"))
		gotest.NoError(w, err)
		allow, err := strconv.Atoi(strings.TrimSpace(string(data)))
		gotest.NoError(w, err)
		w.It("prints no more clauses than tests/drill/unmutated-allow.txt allows", func(it *gotest.T) {
			gotest.LessOrEqual(it, len(got), allow)
		})
	})
}
