package conventions_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ReadmeExampleTestSuite: the README's first example is kept as a fixture
// package, runs green as written, and the README shows what it prints. It
// guards this repository's README, not gotest's behavior, so it is a
// convention rather than a scenario in specs/.
type ReadmeExampleTestSuite struct{}

func (s *ReadmeExampleTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// durations replaces every "(12ms)", "(<1ms)" or "(0.01s)" so two runs compare.
var durations = regexp.MustCompile(`\((<1ms|\d+(\.\d+)?(ms|s))\)`)

// readmeExample returns the fenced blocks of the README's first example, in
// order: the suite, the run command, its output, the spec command, its output.
func readmeExample(t *gotest.T) []string {
	data, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	gotest.NoError(t, err)
	_, section, found := strings.Cut(string(data), "\n## 30-Second Example\n")
	gotest.True(t, found, "README has no 30-Second Example section")
	section, _, _ = strings.Cut(section, "\n## ")
	var blocks []string
	for _, part := range strings.Split(section, "\n```")[1:] {
		_, body, _ := strings.Cut(part, "\n")
		blocks = append(blocks, body)
	}
	// Fences come in pairs: an opening fence's body is a block, a closing
	// fence's "body" is the prose that follows it.
	var out []string
	for i := 0; i < len(blocks); i += 2 {
		out = append(out, strings.TrimRight(blocks[i], "\n"))
	}
	gotest.Len(t, out, 5, "the example shows the suite, a run, its output, a spec run and its output")
	return out
}

func (s *ReadmeExampleTestSuite) TestTheFirstExampleIsTheFixture(t *gotest.T) {
	blocks := readmeExample(t)
	fixture, err := os.ReadFile(filepath.Join("testdata", "readme", "user_service_suite_test.go"))
	gotest.NoError(t, err)
	shown := strings.Join(strings.Split(blocks[0], "\n")[1:], "\n") // drop the file-name comment
	shown = strings.Replace(shown, "package user\n", "package readme\n", 1)
	t.When("the README shows its first suite", func(w *gotest.T) {
		w.It("is the suite the fixture package holds, to the line", func(it *gotest.T) {
			// The README indents with four spaces where gofmt writes a tab.
			gotest.Equal(it, strings.TrimSpace(shown), strings.TrimSpace(strings.ReplaceAll(string(fixture), "\t", "    ")))
		})
	})
}

func (s *ReadmeExampleTestSuite) TestTheFirstExampleRunsAsWritten(t *gotest.T) {
	blocks := readmeExample(t)
	run, err := testkit.RunCLI(t.Context(), ".", "./testdata/readme/", "-v")
	gotest.NoError(t, err)
	t.When("the README's first example runs with -v", func(w *gotest.T) {
		w.It("passes as written", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
		})
		w.It("prints every line the README shows for it", func(it *gotest.T) {
			got := durations.ReplaceAllString(run.Stdout, "(…)")
			for _, line := range strings.Split(durations.ReplaceAllString(blocks[2], "(…)"), "\n") {
				gotest.Contains(it, got, line)
			}
		})
	})
}

func (s *ReadmeExampleTestSuite) TestTheFirstExampleRendersTheSpecShown(t *gotest.T) {
	blocks := readmeExample(t)
	run, err := testkit.RunCLI(t.Context(), ".", "spec", "--no-color", "./testdata/readme/")
	gotest.NoError(t, err)
	t.When("the README's first example renders as a spec", func(w *gotest.T) {
		w.It("prints the tree the README shows, durations aside", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stderr: %s", run.Stderr)
			want := durations.ReplaceAllString(strings.TrimSpace(blocks[4]), "(…)")
			got := durations.ReplaceAllString(strings.TrimSpace(run.Stdout), "(…)")
			gotest.Equal(it, want, got)
		})
	})
}
