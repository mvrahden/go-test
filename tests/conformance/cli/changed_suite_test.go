package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIChangedTestSuite: --changed runs what the record does not hold as
// current, says how far it narrowed, and the record keeps every package.
type CLIChangedTestSuite struct{}

func (s *CLIChangedTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: time.Minute} // up to four runs of a staged module
}

// ranPackages are the fresh packages a text run printed a verdict line of.
func ranPackages(stdout string) []string {
	var out []string
	for _, pkg := range []string{"fresh/a", "fresh/b", "fresh/c", "fresh/d"} {
		for line := range strings.Lines(stdout) {
			if strings.HasPrefix(line, "ok  \t"+pkg+"\t") || strings.HasPrefix(line, "FAIL\t"+pkg+"\t") {
				out = append(out, pkg)
				break
			}
		}
	}
	return out
}

// streamPackages are the packages a stream holds a verdict of.
func streamPackages(t *gotest.T, stdout string) []string {
	hashes, _ := verdictHashes(t, stdout)
	var out []string
	for _, pkg := range []string{"fresh/a", "fresh/b", "fresh/c", "fresh/d"} {
		if _, ok := hashes[pkg]; ok {
			out = append(out, pkg)
		}
	}
	return out
}

// recordedPackage is a package's entry in a run's record.
type recordedPackage struct {
	Package   string
	Verdict   string
	InputHash string
	Elapsed   float64
	Narrowed  bool
}

// lastRecord reads the one last.json of the tree's cache.
func (tr freshTree) lastRecord(t *gotest.T) map[string]recordedPackage {
	files, err := filepath.Glob(filepath.Join(tr.cache, "runs", "*", "last.json"))
	gotest.NoError(t, err)
	gotest.Len(t, files, 1)
	data, err := os.ReadFile(files[0])
	gotest.NoError(t, err)
	var rec struct{ Packages []recordedPackage }
	gotest.NoError(t, json.Unmarshal(data, &rec))
	out := map[string]recordedPackage{}
	for _, p := range rec.Packages {
		out[p.Package] = p
	}
	return out
}

func (s *CLIChangedTestSuite) TestARunAfterAChange(t *gotest.T) {
	tr := stageFresh(t)
	gotest.Equal(t, 1, tr.run(t, "./...").Exit)
	tr.edit(t, "b/b.go", "\n// edited\n")
	text := tr.run(t, "-v", "--changed", "./...")
	stream := tr.run(t, "-json", "--changed", "./...")
	spec := tr.run(t, "spec", "--changed", "--no-color", "./...")

	t.When("--changed follows a run, after a file two of its packages read changed", func(w *gotest.T) {
		w.It("runs those two packages and the one that failed, and not the one unchanged", func(it *gotest.T) {
			gotest.Equal(it, 1, text.Exit, "stderr: %s", text.Stderr)
			gotest.Equal(it, []string{"fresh/a", "fresh/b", "fresh/d"}, ranPackages(text.Stdout), "stdout: %s", text.Stdout)
		})
		w.It("-json and spec --changed hold no verdict of the unchanged package", func(it *gotest.T) {
			gotest.Equal(it, []string{"fresh/d"}, streamPackages(it, stream.Stdout))
			gotest.Contains(it, spec.Stdout, "=== d")
			gotest.NotContains(it, spec.Stdout, "=== c")
		})
	})
}

func (s *CLIChangedTestSuite) TestAfterANarrowedRun(t *gotest.T) {
	tr := stageFresh(t)
	gotest.Equal(t, 1, tr.run(t, "./...").Exit)
	tr.edit(t, "a/a_suite_test.go", "\n// edited\n")
	narrowed := tr.run(t, "--changed", "-run=TestATestSuite/TestTwo", "./...")
	again := tr.run(t, "--changed", "./...")

	t.When("--changed follows a --changed run that -run narrowed", func(w *gotest.T) {
		w.It("runs again the packages the narrowed run ran, and none it left out", func(it *gotest.T) {
			gotest.Equal(it, []string{"fresh/a", "fresh/d"}, ranPackages(narrowed.Stdout), "stdout: %s", narrowed.Stdout)
			gotest.Equal(it, []string{"fresh/a", "fresh/d"}, ranPackages(again.Stdout), "stdout: %s", again.Stdout)
		})
	})
}

func (s *CLIChangedTestSuite) TestWithoutARecord(t *gotest.T) {
	tr := stageFresh(t)
	run := tr.run(t, "--changed", "./...")

	t.When("--changed is given in a working directory without a record", func(w *gotest.T) {
		w.It("runs every package and says on stderr that no run is recorded", func(it *gotest.T) {
			gotest.Equal(it, []string{"fresh/a", "fresh/b", "fresh/c", "fresh/d"}, ranPackages(run.Stdout), "stdout: %s", run.Stdout)
			gotest.Contains(it, run.Stderr, "gotest: --changed: no run of "+tr.dir+" is recorded, so every package runs\n")
			gotest.Contains(it, run.Stderr, "| changed only: no record, all ran\n")
		})
	})
}

func (s *CLIChangedTestSuite) TestBesideFailed(t *gotest.T) {
	tr := stageFresh(t)
	gotest.Equal(t, 1, tr.run(t, "./...").Exit)
	run := tr.run(t, "-json", "--changed", "--failed", "./...")

	t.When("--changed --failed follows a run in which one method of an unchanged package failed", func(w *gotest.T) {
		w.It("reruns only that method", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stderr: %s", run.Stderr)
			gotest.Equal(it, []string{"TestDTestSuite", "TestDTestSuite/TestFails"}, testsRun(&run))
		})
	})
}

func (s *CLIChangedTestSuite) TestARunThatLeftPackagesOut(t *gotest.T) {
	tr := stageFresh(t)
	gotest.Equal(t, 0, tr.run(t, "./a/", "./b/", "./c/").Exit)
	tr.edit(t, "b/b.go", "\n// edited\n")
	text := tr.run(t, "--changed", "./a/", "./b/", "./c/")
	tr.edit(t, "b/b.go", "// again\n")
	stream := tr.run(t, "-json", "--changed", "./a/", "./b/", "./c/")
	first, last := streamEnds(t, stream.Stdout)
	var opened struct {
		Manifest struct {
			Changed *struct{ Ran, Unchanged int }
		}
	}
	gotest.NoError(t, json.Unmarshal([]byte(strings.SplitN(stream.Stdout, "\n", 2)[0]), &opened))

	t.When("a --changed run leaves packages out as unchanged", func(w *gotest.T) {
		w.It("ends stderr with a closing line counting the packages it ran and those unchanged", func(it *gotest.T) {
			gotest.Equal(it, 0, text.Exit, "stderr: %s", text.Stderr)
			gotest.Equal(it, "gotest: PASS exit 0 | packages: 2 passed, 0 failed, 0 skipped | changed only: 2 ran, 1 unchanged", lastLine(text.Stderr))
		})
		w.It("opens its -json stream with a manifest whose Changed holds both counts", func(it *gotest.T) {
			gotest.Equal(it, "gotest run", first.Package)
			gotest.NotZero(it, opened.Manifest.Changed)
			gotest.Equal(it, struct{ Ran, Unchanged int }{2, 1}, *opened.Manifest.Changed)
		})
		w.It("closes the stream with 0 units not run", func(it *gotest.T) {
			gotest.NotZero(it, last.Result)
			gotest.NotZero(it, last.Result.NotRun)
			gotest.Equal(it, 0, *last.Result.NotRun)
		})
	})
}

func (s *CLIChangedTestSuite) TestThePackagesOfANarrowedRun(t *gotest.T) {
	tr := stageFresh(t)
	gotest.Equal(t, 0, tr.run(t, "-run=TestBTestSuite", "./b/").Exit)
	rec := tr.lastRecord(t)

	t.When("a -run narrowed run of a package ends", func(w *gotest.T) {
		w.It("its record lists the package with its verdict, its InputHash and its time, marked narrowed", func(it *gotest.T) {
			gotest.Len(it, rec, 1)
			b := rec["fresh/b"]
			gotest.Equal(it, "pass", b.Verdict)
			gotest.Regexp(it, "^[0-9a-f]{32}$", b.InputHash)
			gotest.Greater(it, b.Elapsed, 0.0)
			gotest.True(it, b.Narrowed)
		})
	})
}

func (s *CLIChangedTestSuite) TestTheRecordAfterAChangedRun(t *gotest.T) {
	tr := stageFresh(t)
	gotest.Equal(t, 0, tr.run(t, "./a/", "./b/", "./c/").Exit)
	before := tr.lastRecord(t)
	tr.edit(t, "b/b.go", "\n// edited\n")
	gotest.Equal(t, 0, tr.run(t, "--changed", "./a/", "./b/", "./c/").Exit)
	after := tr.lastRecord(t)

	t.When("a --changed run leaves out packages the record before it held", func(w *gotest.T) {
		w.It("the record keeps their verdicts and hashes beside those of the packages it ran", func(it *gotest.T) {
			gotest.Len(it, after, 3)
			gotest.Equal(it, before["fresh/c"], after["fresh/c"])
			gotest.NotEqual(it, before["fresh/a"].InputHash, after["fresh/a"].InputHash)
			gotest.NotEqual(it, before["fresh/b"].InputHash, after["fresh/b"].InputHash)
			gotest.Equal(it, "pass", after["fresh/b"].Verdict)
		})
	})
}
