package conventions_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/speccatalog"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ReadmeExampleTestSuite: the README's first example is kept as a fixture
// package, runs green as written, and the README shows what it prints. It
// guards this repository's README, not gotest's behavior, so it is a
// convention rather than a scenario in specs/.
type ReadmeExampleTestSuite struct{}

func (s *ReadmeExampleTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 45 * time.Second}
}

// durations replaces every "(12ms)", "(<1ms)" or "(0.01s)" so two runs compare.
var durations = regexp.MustCompile(`\((<1ms|\d+(\.\d+)?(ms|s))\)`)

var (
	provenanceLine = regexp.MustCompile(`(?m)^Runs from [^\n]*\n\n`)
	packageHeading = regexp.MustCompile(`(?m)^=== \S+ — `)
)

// readmeExample returns the fenced blocks of the README's suite example, in
// order: the suite, its production code, the run command, its output, the
// spec command, its output.
func readmeExample(t *gotest.T) []string {
	out := readmeBlocks(t, "\n### Step 2: Write a Suite\n", "README has no Step 2: Write a Suite section")
	gotest.Len(t, out, 6, "the example shows the suite, its production code, a run, its output, a spec run and its output")
	return out
}

// readmeBlocks returns the bodies of the fenced blocks of the README section
// that opens with heading and ends at the next heading of level two or three.
func readmeBlocks(t *gotest.T, heading, missing string) []string {
	data, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	gotest.NoError(t, err)
	_, section, found := strings.Cut(string(data), heading)
	gotest.True(t, found, missing)
	section, _, _ = strings.Cut(section, "\n## ")
	section, _, _ = strings.Cut(section, "\n### ")
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
	return out
}

// asShown is a fixture file as the README prints it, from its package line
// on: the README indents with four spaces where gofmt writes a tab.
func asShown(t *gotest.T, path string) string {
	data, err := os.ReadFile(path) //nolint:gosec // G304: the suite's own fixture
	gotest.NoError(t, err)
	text := string(data)
	if i := strings.Index(text, "\npackage "); i >= 0 && !strings.HasPrefix(text, "package ") {
		text = text[i+1:]
	}
	return strings.TrimSpace(strings.ReplaceAll(text, "\t", "    "))
}

// dropNameLine drops a block's first line, the file-name comment.
func dropNameLine(block string) string {
	return strings.Join(strings.Split(block, "\n")[1:], "\n")
}

func (s *ReadmeExampleTestSuite) TestTheFirstExampleIsTheFixture(t *gotest.T) {
	blocks := readmeExample(t)
	suite := strings.Replace(dropNameLine(blocks[0]), "package user\n", "package readme\n", 1)
	code := strings.Replace(dropNameLine(blocks[1]), "package user\n", "package readme\n", 1)
	t.When("the README shows its first suite", func(w *gotest.T) {
		w.It("is the suite the fixture package holds, to the line", func(it *gotest.T) {
			gotest.Equal(it, strings.TrimSpace(suite), asShown(it, filepath.Join("testdata", "readme", "user_service_suite_test.go")))
		})
		w.It("shows the production code the fixture package holds", func(it *gotest.T) {
			gotest.Equal(it, strings.TrimSpace(code), asShown(it, filepath.Join("testdata", "readme", "user.go")))
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
			// Whole lines: a shortened name must not pass as a prefix of the real one.
			got := strings.Split(durations.ReplaceAllString(run.Stdout, "(…)"), "\n")
			for _, line := range strings.Split(durations.ReplaceAllString(blocks[3], "(…)"), "\n") {
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
			want := durations.ReplaceAllString(strings.TrimSpace(blocks[5]), "(…)")
			got := durations.ReplaceAllString(strings.TrimSpace(run.Stdout), "(…)")
			// The README names a package of its own and no checkout or run times.
			got = provenanceLine.ReplaceAllString(got, "")
			got = packageHeading.ReplaceAllString(got, "=== pkg/user — ")
			gotest.Equal(it, want, got)
		})
	})
}

func (s *ReadmeExampleTestSuite) TestTheContentsListEverySection(t *gotest.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	gotest.NoError(t, err)
	var want, got []string
	inFence, inContents := false, false
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if heading, ok := strings.CutPrefix(line, "## "); ok {
			inContents = heading == "Contents"
			if !inContents {
				want = append(want, "- ["+heading+"](#"+speccatalog.Slug(heading)+")")
			}
			continue
		}
		if inContents && strings.HasPrefix(line, "- [") {
			got = append(got, line)
		}
	}
	t.When("the README's contents are compared with its sections", func(w *gotest.T) {
		w.It("list every section once, in order, by the anchor GitHub gives it", func(it *gotest.T) {
			gotest.Equal(it, want, got)
		})
	})
}

func (s *ReadmeExampleTestSuite) TestTheLibraryExampleRunsUnderGoTest(t *gotest.T) {
	blocks := readmeBlocks(t, "\n### Step 1: Use the Library in the Tests You Have\n", "README has no Step 1: Use the Library in the Tests You Have section")
	gotest.NotEmpty(t, blocks)
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	dir := t.TempDir()
	_, err = testkit.StagePackage(root, filepath.Join("testdata", "library"), dir, "librarymod")
	gotest.NoError(t, err)
	cmd := exec.CommandContext(t.Context(), "go", "test", "-count=1", "-v", "./library/")
	cmd.Dir = dir
	cmd.Env = testkit.ChildEnv(dir, "GOFLAGS=")
	out, err := cmd.CombinedOutput()
	t.When("the README's library example runs under plain go test", func(w *gotest.T) {
		w.It("is the test the fixture package holds, to the line", func(it *gotest.T) {
			gotest.Equal(it, strings.TrimSpace(dropNameLine(blocks[0])), asShown(it, filepath.Join("testdata", "library", "price_test.go")))
		})
		w.It("passes", func(it *gotest.T) {
			gotest.NoError(it, err, "output: %s", out)
		})
		w.It("runs the nested behavior and each row as subtests", func(it *gotest.T) {
			gotest.Contains(it, string(out), "--- PASS: TestTotal/the_cart_is_empty/totals_zero")
			gotest.Contains(it, string(out), "--- PASS: TestTotal/one_item")
			gotest.Contains(it, string(out), "--- PASS: TestTotal/two_items")
		})
	})
}
