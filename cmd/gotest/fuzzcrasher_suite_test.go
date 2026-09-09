package main_test

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// FuzzCrasherLoopTestSuite drives the whole crasher loop on the staged
// fuzzcrash module: a real session must find the crash, report the new
// corpus file and exit 1; triage must reproduce it; promote must splice it
// as a seed that then replays as an ordinary failure. Exclusive because the
// session runs the engine's workers.
//
//nolint:lifecycle-pair // BeforeAll's binary lives under t.TempDir(), which the framework removes automatically
type FuzzCrasherLoopTestSuite struct {
	binary   string
	repoRoot string
	pkgDir   string
}

func (s *FuzzCrasherLoopTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Exclusive: true}
}

func (s *FuzzCrasherLoopTestSuite) BeforeAll(t *gotest.T) {
	absRoot, err := filepath.Abs("../..")
	gotest.NoError(t, err)
	s.repoRoot = absRoot
	s.binary = buildGotestBinary(t, absRoot, t.TempDir())
}

func (s *FuzzCrasherLoopTestSuite) BeforeEach(t *gotest.T) {
	s.pkgDir = stageFuzzModule(t, s.repoRoot, t.TempDir())
}

func (s *FuzzCrasherLoopTestSuite) TestCrasherLoop(t *gotest.T) {
	const target = "FuzzMessageTestSuite_FuzzOnlySeed"
	corpusDir := filepath.Join(s.pkgDir, "testdata", "fuzz", target)

	t.When("a session finds a crasher", func(w *gotest.T) {
		summaryPath := filepath.Join(w.TempDir(), "summary.md")
		env := []string{"GITHUB_ACTIONS=true", "GITHUB_STEP_SUMMARY=" + summaryPath}
		// No --for: the default budget applies, and the crash ends the
		// session long before it runs out.
		out, code := runGotestIn(w, s.binary, s.pkgDir, env, "fuzz", "--target="+target, ".")

		w.It("exits 1", func(it *gotest.T) {
			gotest.Equal(it, 1, code, "session output:\n%s", out)
		})
		w.It("ran under the default budget and said so", func(it *gotest.T) {
			gotest.Contains(it, out, "1m0s each (~1m0s wall-clock, hard stop at 3m0s; --for defaulted to 1m0s, --for=0 removes the budget)")
		})
		w.It("names the new corpus file and the commands that act on it", func(it *gotest.T) {
			gotest.Regexp(it, `\[`+target+`\] new crasher: .*`+regexpPath("testdata/fuzz/"+target)+`/`, out)
			gotest.Contains(it, out, "gotest fuzz triage")
			gotest.Contains(it, out, "gotest fuzz promote")
		})
		w.It("counts it in the closing line and the step summary", func(it *gotest.T) {
			gotest.Regexp(it, `fuzzed 1 target in \d+\.\ds: [\d,]+ execs, \d+ new interesting inputs?, 1 new crasher \(`+target+`\)`, out)
			summary, err := os.ReadFile(summaryPath)
			gotest.NoError(it, err)
			gotest.Contains(it, string(summary), "1 new crasher")
		})
		w.It("left exactly one corpus entry on disk", func(it *gotest.T) {
			entries, err := os.ReadDir(corpusDir)
			gotest.NoError(it, err)
			gotest.Len(it, entries, 1)
		})

		w.When("the crasher is triaged", func(w *gotest.T) {
			out, code := runGotestIn(w, s.binary, s.pkgDir, nil, "fuzz", "triage", ".")
			w.It("reproduces with the decoded input and the cause", func(it *gotest.T) {
				gotest.Equal(it, 1, code, "triage output:\n%s", out)
				gotest.Contains(it, out, "input:")
				gotest.Contains(it, out, "unexpected input")
			})
		})

		w.When("the crasher is promoted", func(w *gotest.T) {
			out, code := runGotestIn(w, s.binary, s.pkgDir, nil, "fuzz", "promote", ".")
			w.It("splices a second seed and removes the corpus file", func(it *gotest.T) {
				gotest.Equal(it, 0, code, "promote output:\n%s", out)
				src, err := os.ReadFile(filepath.Join(s.pkgDir, "suite_test.go"))
				gotest.NoError(it, err)
				// The fixture's three targets carry one seed each; promote adds one.
				gotest.Equal(it, 4, strings.Count(string(src), "f.Add("))
				entries, _ := os.ReadDir(corpusDir)
				gotest.Empty(it, entries)
			})
			w.It("replays the promoted seed as an ordinary failing subtest", func(it *gotest.T) {
				out, code := runGotestIn(w, s.binary, s.pkgDir, nil, ".")
				gotest.Equal(it, 1, code, "replay output:\n%s", out)
				gotest.Contains(it, out, target+"/seed#1")
				gotest.Contains(it, out, "unexpected input")
			})
		})
	})
}

// regexpPath escapes a slash-separated path for a regexp and lets either
// separator match, so the expectation holds on Windows too.
func regexpPath(p string) string {
	out := ""
	for _, r := range p {
		switch r {
		case '/':
			out += `[\\/]`
		case '.':
			out += `\.`
		default:
			out += string(r)
		}
	}
	return out
}
