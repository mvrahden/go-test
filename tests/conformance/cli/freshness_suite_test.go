package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIInputHashTestSuite: a verdict says which source it judged, and a saved
// stream is told current or stale against the tree.
type CLIInputHashTestSuite struct{}

func (s *CLIInputHashTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: time.Minute} // a staged module run up to three times, checked up to five
}

// freshTree is a staged copy of testdata/fresh with a gotest cache of its
// own: a imports b, c embeds a file and reads its testdata and a file
// outside it, d has a failing method.
type freshTree struct {
	dir, cache string
}

func stageFresh(t *gotest.T) freshTree {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	tr := freshTree{dir: filepath.Join(t.TempDir(), "fresh"), cache: t.TempDir()}
	gotest.NoError(t, testkit.StageModule(root, filepath.Join("..", "testdata", "fresh"), tr.dir))
	return tr
}

// run runs the CLI in the tree with args.
func (tr freshTree) run(t *gotest.T, args ...string) testkit.Run {
	run, err := testkit.RunCLIWithEnv(t.Context(), tr.dir, inCache(tr.cache), args...)
	gotest.NoError(t, err)
	return run
}

// stream runs args with -json and saves the stream to a file it returns.
func (tr freshTree) stream(t *gotest.T, args ...string) (string, testkit.Run) {
	run := tr.run(t, append([]string{"-json"}, args...)...)
	file := filepath.Join(t.TempDir(), "events.json")
	gotest.NoError(t, os.WriteFile(file, []byte(run.Stdout), 0o600))
	return file, run
}

// edit appends text to the tree's file rel and returns the undo.
func (tr freshTree) edit(t *gotest.T, rel, text string) func() {
	path := filepath.Join(tr.dir, filepath.FromSlash(rel))
	before, err := os.ReadFile(path)
	gotest.NoError(t, err)
	gotest.NoError(t, os.WriteFile(path, append(slices.Clone(before), text...), 0o600))
	return func() { gotest.NoError(t, os.WriteFile(path, before, 0o600)) }
}

// checkedDoc is the part of spec --check-fresh --format=json the suites read.
type checkedDoc struct {
	Packages []struct {
		Path      string `json:"path"`
		InputHash string `json:"inputHash"`
		Freshness string `json:"freshness"`
	} `json:"packages"`
	Stats struct {
		Stale int `json:"stale"`
	} `json:"stats"`
}

func (tr freshTree) check(t *gotest.T, file string) checkedDoc {
	run := tr.run(t, "spec", "--input="+file, "--check-fresh", "--format=json", "--render-only")
	gotest.Equal(t, 0, run.Exit, "stderr: %s", run.Stderr)
	var doc checkedDoc
	gotest.NoError(t, json.Unmarshal([]byte(run.Stdout), &doc), "document: %s", run.Stdout)
	return doc
}

// freshness is each package's freshness in doc.
func (d *checkedDoc) freshness() map[string]string {
	out := map[string]string{}
	for _, p := range d.Packages {
		out[p.Path] = p.Freshness
	}
	return out
}

// verdictHashes are the InputHash of each package's verdict event in a
// stream, and the test events that carry one.
func verdictHashes(t *gotest.T, stdout string) (hashes map[string]string, onTests []string) {
	hashes = map[string]string{}
	for line := range strings.Lines(stdout) {
		var ev map[string]any
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		pkg, _ := ev["Package"].(string)
		test, _ := ev["Test"].(string)
		action, _ := ev["Action"].(string)
		hash, has := ev["InputHash"].(string)
		switch {
		case ev["RunLevel"] == true:
		case test != "" && has:
			onTests = append(onTests, pkg+" "+test)
		case test == "" && (action == "pass" || action == "fail" || action == "skip"):
			hashes[pkg] = hash
		}
	}
	gotest.NotEmpty(t, hashes, "stream: %s", stdout)
	return hashes, onTests
}

func (s *CLIInputHashTestSuite) TestAPackagesInputHash(t *gotest.T) {
	tr := stageFresh(t)
	file, run := tr.stream(t, "-tags=fresh", "./...")
	gotest.Equal(t, 1, run.Exit, "stderr: %s", run.Stderr)
	hashes, onTests := verdictHashes(t, run.Stdout)
	spec := tr.run(t, "spec", "--input="+file, "--format=json", "--render-only")
	var doc checkedDoc
	gotest.NoError(t, json.Unmarshal([]byte(spec.Stdout), &doc), "document: %s", spec.Stdout)

	t.When("a -json run is made with -tags", func(w *gotest.T) {
		w.It("stamps each package's verdict event with an InputHash and no test event with one", func(it *gotest.T) {
			gotest.Len(it, hashes, 4)
			for pkg, h := range hashes {
				gotest.Regexp(it, "^[0-9a-f]{32}$", h, pkg)
			}
			gotest.Empty(it, onTests)
		})
		w.It("spec --format=json of the stream gives each package its inputHash", func(it *gotest.T) {
			got := map[string]string{}
			for _, p := range doc.Packages {
				got[p.Path] = p.InputHash
				gotest.Empty(it, p.Freshness, p.Path)
			}
			gotest.Equal(it, hashes, got)
		})
	})
}

func (s *CLIInputHashTestSuite) TestEditingWhatAPackageReads(t *gotest.T) {
	tr := stageFresh(t)
	file, run := tr.stream(t, "./a/", "./b/", "./c/")
	gotest.Equal(t, 0, run.Exit, "stderr: %s", run.Stderr)
	edits := []struct{ desc, file, text string }{
		{"a dependency's file", "b/b.go", "\n// edited\n"},
		{"an embedded file", "c/data.txt", "edited\n"},
		{"a testdata file", "c/testdata/fixture.txt", "edited\n"},
	}
	got := map[string]map[string]string{}
	for _, e := range edits {
		undo := tr.edit(t, e.file, e.text)
		doc := tr.check(t, file)
		got[e.desc] = doc.freshness()
		undo()
	}

	t.When("a file of a package's dependency in its module, a file it embeds or a file of its testdata changes after a run", func(w *gotest.T) {
		w.It("spec --check-fresh of the run's stream marks that package stale", func(it *gotest.T) {
			gotest.Equal(it, "stale", got["a dependency's file"]["fresh/a"])
			gotest.Equal(it, "stale", got["a dependency's file"]["fresh/b"])
			gotest.Equal(it, "stale", got["an embedded file"]["fresh/c"])
			gotest.Equal(it, "stale", got["a testdata file"]["fresh/c"])
		})
		w.It("marks current a package that reads none of them", func(it *gotest.T) {
			gotest.Equal(it, "current", got["a dependency's file"]["fresh/c"])
			gotest.Equal(it, map[string]string{"fresh/a": "current", "fresh/b": "current", "fresh/c": "stale"}, got["an embedded file"])
			gotest.Equal(it, "current", got["a testdata file"]["fresh/a"])
		})
	})
}

func (s *CLIInputHashTestSuite) TestOtherBuildFlags(t *gotest.T) {
	tr := stageFresh(t)
	plain := tr.run(t, "-json", "./b/")
	tagged := tr.run(t, "-json", "-tags=fresh", "./b/")
	plainHashes, _ := verdictHashes(t, plain.Stdout)
	taggedHashes, _ := verdictHashes(t, tagged.Stdout)

	t.When("one package is run with and without -tags", func(w *gotest.T) {
		w.It("its two verdicts carry two hashes", func(it *gotest.T) {
			gotest.NotEmpty(it, plainHashes["fresh/b"])
			gotest.NotEmpty(it, taggedHashes["fresh/b"])
			gotest.NotEqual(it, plainHashes["fresh/b"], taggedHashes["fresh/b"])
		})
	})
}

func (s *CLIInputHashTestSuite) TestAFileTheHashDoesNotCover(t *gotest.T) {
	tr := stageFresh(t)
	file, run := tr.stream(t, "-tags=fresh", "./c/")
	gotest.Equal(t, 0, run.Exit, "stderr: %s", run.Stderr)
	first, _ := streamEnds(t, run.Stdout)
	var manifest struct{ Manifest struct{ Build []string } }
	gotest.NoError(t, json.Unmarshal([]byte(strings.SplitN(run.Stdout, "\n", 2)[0]), &manifest))
	tr.edit(t, "notes/outside.txt", "edited\n")
	doc := tr.check(t, file)

	t.When("a file the package's test reads outside its testdata trees changes after a run", func(w *gotest.T) {
		w.It("spec --check-fresh of the run's stream marks the package current", func(it *gotest.T) {
			gotest.Equal(it, map[string]string{"fresh/c": "current"}, doc.freshness())
		})
		w.It("the run's manifest names its -tags in Build", func(it *gotest.T) {
			gotest.Equal(it, "gotest run", first.Package)
			gotest.Equal(it, []string{"-tags=fresh"}, manifest.Manifest.Build)
		})
	})
}

func (s *CLIInputHashTestSuite) TestAStreamCheckedAgainstTheTree(t *gotest.T) {
	tr := stageFresh(t)
	file, run := tr.stream(t, "./a/", "./b/", "./c/")
	gotest.Equal(t, 0, run.Exit, "stderr: %s", run.Stderr)
	goTest := filepath.Join(t.TempDir(), "gotest.json")
	gotest.NoError(t, os.WriteFile(goTest, []byte(strings.Join([]string{
		`{"Action":"start","Package":"fresh/b"}`,
		`{"Action":"run","Package":"fresh/b","Test":"TestOne"}`,
		`{"Action":"pass","Package":"fresh/b","Test":"TestOne","Elapsed":0.01}`,
		`{"Action":"pass","Package":"fresh/b","Elapsed":0.01}`,
	}, "\n")+"\n"), 0o600))
	tr.edit(t, "c/data.txt", "edited\n")
	doc := tr.check(t, file)
	plain := tr.check(t, goTest)
	terminal := tr.run(t, "spec", "--input="+file, "--check-fresh", "--no-color")
	markdown := tr.run(t, "spec", "--input="+file, "--check-fresh", "--format=md")
	summary := tr.run(t, "summary", "--input="+file, "--check-fresh", "--no-color")
	unchecked := tr.run(t, "spec", "--input="+file, "--no-color")

	t.When("spec --check-fresh reads a run's stream after one of its packages changed, and go test's own stream", func(w *gotest.T) {
		w.It("its JSON marks the changed package stale and the others current, counting 1 stale", func(it *gotest.T) {
			gotest.Equal(it, map[string]string{"fresh/a": "current", "fresh/b": "current", "fresh/c": "stale"}, doc.freshness())
			gotest.Equal(it, 1, doc.Stats.Stale)
		})
		w.It("marks the package of go test's stream unknown", func(it *gotest.T) {
			gotest.Equal(it, map[string]string{"fresh/b": "unknown"}, plain.freshness())
			gotest.Zero(it, plain.Stats.Stale)
		})
		w.It("the terminal and Markdown spec and the summary mark the stale package, and exit as without --check-fresh", func(it *gotest.T) {
			gotest.Contains(it, terminal.Stdout, "=== c — STALE: its inputs changed since the run")
			gotest.NotContains(it, terminal.Stdout, "=== b — STALE")
			gotest.Contains(it, markdown.Stdout, "c — STALE: its inputs changed since the run")
			gotest.Contains(it, summary.Stdout, "STALE  fresh/c — its inputs changed since the run")
			gotest.NotContains(it, summary.Stdout, "fresh/a — its inputs")
			gotest.NotContains(it, unchecked.Stdout, "STALE")
			gotest.Equal(it, []int{unchecked.Exit, unchecked.Exit, unchecked.Exit}, []int{terminal.Exit, markdown.Exit, summary.Exit})
		})
	})
}

func (s *CLIInputHashTestSuite) TestALiveRunAskedToCheck(t *gotest.T) {
	t.When("spec or summary is given --check-fresh without --input", func(w *gotest.T) {
		w.It("refuses with exit 2, naming --input", func(it *gotest.T) {
			for sub, cmd := range gotest.Each(it, []struct{ Desc string }{{Desc: "spec"}, {Desc: "summary"}}) {
				run, err := testkit.RunCLIWithEnv(sub.Context(), ".", inCache(sub.TempDir()), cmd.Desc, "--check-fresh", "../testdata/passing/")
				gotest.NoError(sub, err)
				gotest.Equal(sub, 2, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				gotest.Empty(sub, run.Stdout)
				gotest.Contains(sub, run.Stderr, "--check-fresh judges a saved stream against the working tree and requires --input")
			}
		})
	})
}
