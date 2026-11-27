package cli_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIRunRecordsTestSuite: each run keeps its record in a bounded history
// that gotest runs lists, a saved stream among them.
type CLIRunRecordsTestSuite struct{}

func (s *CLIRunRecordsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: time.Minute} // up to three runs of a staged module
}

// history lists the files of the tree's history, oldest first.
func (tr freshTree) history(t *gotest.T) []string {
	files, err := filepath.Glob(filepath.Join(tr.cache, "runs", "*", "history", "*.json"))
	gotest.NoError(t, err)
	return files
}

// keptRecord is the part of a kept record the suites read.
type keptRecord struct {
	EventsFile string
	Packages   []struct{ Package string }
}

func readKept(t *gotest.T, file string) keptRecord {
	data, err := os.ReadFile(file)
	gotest.NoError(t, err)
	var rec keptRecord
	gotest.NoError(t, json.Unmarshal(data, &rec))
	return rec
}

func (s *CLIRunRecordsTestSuite) TestThreeRuns(t *gotest.T) {
	tr := stageFresh(t)
	events := filepath.Join(t.TempDir(), "saved", "events.json")
	gotest.Equal(t, 0, tr.run(t, "./b/").Exit)
	gotest.Equal(t, 0, tr.run(t, "-json", "./b/").Exit)
	gotest.Equal(t, 0, tr.run(t, "-json", "--events-file="+events, "./b/", "./c/").Exit)
	var kept []keptRecord
	for _, f := range tr.history(t) {
		kept = append(kept, readKept(t, f))
	}
	clean := tr.run(t, "clean", "--cache")
	after := tr.history(t)
	last, err := filepath.Glob(filepath.Join(tr.cache, "runs", "*", "last.json"))
	gotest.NoError(t, err)

	t.When("three runs are made in one working directory, one with --events-file", func(w *gotest.T) {
		w.It("the history holds a record of each, the third naming its events file", func(it *gotest.T) {
			gotest.Len(it, kept, 3)
			gotest.Empty(it, kept[0].EventsFile)
			gotest.Len(it, kept[1].Packages, 1)
			third := kept[2]
			gotest.Equal(it, events, third.EventsFile)
			gotest.Len(it, third.Packages, 2)
		})
		w.It("gotest clean --cache removes them", func(it *gotest.T) {
			gotest.Equal(it, 0, clean.Exit, "stderr: %s", clean.Stderr)
			gotest.Contains(it, clean.Stderr, "removed the run records of 1 checkout from ")
			gotest.Empty(it, after)
			gotest.Empty(it, last)
		})
	})
}

// seedHistory writes n records into the history of the run in dir, the
// first old days ago and the others minutes apart, an hour back.
func seedHistory(t *gotest.T, history string, n int, old time.Duration) {
	now := time.Now().UTC()
	for i := range n {
		at := now.Add(-time.Hour + time.Duration(i)*time.Second)
		if i == 0 {
			at = now.Add(-old)
		}
		name := fmt.Sprintf("%s-1-%08d.json", at.Format("20060102T150405.000000000Z"), i)
		gotest.NoError(t, os.WriteFile(filepath.Join(history, name), []byte("{}\n"), 0o600))
	}
}

func (s *CLIRunRecordsTestSuite) TestAFullHistory(t *gotest.T) {
	full, aged := stageFresh(t), stageFresh(t)
	for _, tr := range []freshTree{full, aged} {
		gotest.Equal(t, 0, tr.run(t, "./b/").Exit)
	}
	seedHistory(t, filepath.Dir(full.history(t)[0]), 49, 2*time.Hour)
	seedHistory(t, filepath.Dir(aged.history(t)[0]), 2, 31*24*time.Hour)
	fullBefore := full.history(t)
	agedOld := aged.history(t)[0]
	for _, tr := range []freshTree{full, aged} {
		gotest.Equal(t, 0, tr.run(t, "./b/").Exit)
	}
	fullAfter := full.history(t)

	t.When("a run is recorded beside 50 kept records, and in another directory beside one kept over 30 days", func(w *gotest.T) {
		w.It("the first history holds the newest 50", func(it *gotest.T) {
			gotest.Len(it, fullBefore, 50)
			gotest.Len(it, fullAfter, 50)
			gotest.Equal(it, fullBefore[1:], fullAfter[:49])
		})
		w.It("the second no longer holds the old one", func(it *gotest.T) {
			got := aged.history(it)
			gotest.Len(it, got, 3)
			gotest.NotContains(it, got, agedOld)
		})
	})
}

// runsDoc is the part of runs --format=json the suites read.
type runsDoc struct {
	Version string `json:"version"`
	Runs    []struct {
		Imported   bool   `json:"imported"`
		EventsFile string `json:"eventsFile"`
		Manifest   struct {
			Args []string `json:"args"`
		} `json:"manifest"`
		Result struct {
			ExitCode int             `json:"exitCode"`
			Tests    json.RawMessage `json:"tests"`
		} `json:"result"`
		Failed   []struct{ Package, Test string } `json:"failed"`
		Packages struct {
			Total, Current, Stale, Unknown int
		} `json:"packages"`
		Ran []ranPackage `json:"ran"`
	} `json:"runs"`
}

// ranPackage is a package a listed run judged.
type ranPackage struct {
	Package   string `json:"package"`
	Status    string `json:"status"`
	InputHash string `json:"inputHash"`
}

func (tr freshTree) runs(t *gotest.T, args ...string) (runsDoc, testkit.Run) {
	run := tr.run(t, append([]string{"runs", "--format=json"}, args...)...)
	gotest.Equal(t, 0, run.Exit, "stderr: %s", run.Stderr)
	var doc runsDoc
	gotest.NoError(t, json.Unmarshal([]byte(run.Stdout), &doc), "document: %s", run.Stdout)
	return doc, run
}

func (s *CLIRunRecordsTestSuite) TestTwoRunsListed(t *gotest.T) {
	tr := stageFresh(t)
	gotest.Equal(t, 1, tr.run(t, "./c/", "./d/").Exit)
	first := tr.lastRecord(t)
	tr.edit(t, "c/data.txt", "edited\n")
	gotest.Equal(t, 0, tr.run(t, "./b/").Exit)
	second := tr.lastRecord(t)
	doc, run := tr.runs(t)
	text := tr.run(t, "runs")

	t.When("gotest runs --format=json follows two runs, a package of the first changed since", func(w *gotest.T) {
		w.It("prints one document opening with version that lists both newest first, with manifest, result and failed units", func(it *gotest.T) {
			gotest.Regexp(it, `^\{"version":`, run.Stdout)
			gotest.Len(it, doc.Runs, 2)
			gotest.Equal(it, []string{"./b/"}, doc.Runs[0].Manifest.Args)
			gotest.Equal(it, 0, doc.Runs[0].Result.ExitCode)
			gotest.Empty(it, doc.Runs[0].Failed)
			gotest.Equal(it, []string{"./c/", "./d/"}, doc.Runs[1].Manifest.Args)
			gotest.Equal(it, 1, doc.Runs[1].Result.ExitCode)
			gotest.Equal(it, []struct{ Package, Test string }{{"fresh/d", "TestDTestSuite/TestFails"}}, doc.Runs[1].Failed)
		})
		w.It("counts the changed package of the first stale and every other package current", func(it *gotest.T) {
			gotest.Equal(it, struct{ Total, Current, Stale, Unknown int }{1, 1, 0, 0}, doc.Runs[0].Packages)
			gotest.Equal(it, struct{ Total, Current, Stale, Unknown int }{2, 1, 1, 0}, doc.Runs[1].Packages)
		})
		w.It("lists the packages each run judged, sorted, with their verdict and InputHash", func(it *gotest.T) {
			gotest.Len(it, doc.Runs, 2)
			gotest.Equal(it, []ranPackage{{"fresh/b", "pass", second["fresh/b"].InputHash}}, doc.Runs[0].Ran)
			gotest.Equal(it, []ranPackage{{"fresh/c", "pass", first["fresh/c"].InputHash}, {"fresh/d", "fail", first["fresh/d"].InputHash}}, doc.Runs[1].Ran)
			gotest.NotEmpty(it, first["fresh/c"].InputHash)
		})
		w.It("gives the text runs no test counts", func(it *gotest.T) {
			gotest.Len(it, doc.Runs, 2)
			gotest.Empty(it, doc.Runs[0].Result.Tests)
			gotest.Empty(it, doc.Runs[1].Result.Tests)
			gotest.Contains(it, run.Stdout, `"result":{"exitCode":0,"packages":`)
		})
		w.It("the text format prints one line for each, newest first", func(it *gotest.T) {
			gotest.Equal(it, 0, text.Exit, "stderr: %s", text.Stderr)
			lines := strings.Split(strings.TrimRight(text.Stdout, "\n"), "\n")
			gotest.Len(it, lines, 2)
			gotest.Regexp(it, `\| PASS exit 0 \| packages: 1 current, 0 stale, 0 unknown \| .*gotest \./b/$`, lines[0])
			gotest.Regexp(it, `\| FAIL exit 1 \| packages: 1 current, 1 stale, 0 unknown \| .*gotest \./c/ \./d/$`, lines[1])
		})
	})
}

func (s *CLIRunRecordsTestSuite) TestNothingRecorded(t *gotest.T) {
	tr := stageFresh(t)
	run := tr.run(t, "runs")

	t.When("gotest runs is given in a working directory without a record", func(w *gotest.T) {
		w.It("says no run is recorded and exits 0", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stderr: %s", run.Stderr)
			gotest.Equal(it, "no run of "+tr.dir+" is recorded\n", run.Stdout)
			gotest.Empty(it, run.Stderr)
		})
	})
}

func (s *CLIRunRecordsTestSuite) TestImportingAStream(t *gotest.T) {
	ci, local := stageFresh(t), stageFresh(t)
	saved, _ := ci.stream(t, "./b/", "./c/")
	imported := local.run(t, "runs", "--import="+saved)
	doc, _ := local.runs(t)
	empty := filepath.Join(t.TempDir(), "empty.json")
	gotest.NoError(t, os.WriteFile(empty, []byte(`{"Action":"output","Package":"fresh/b","Output":"x\n"}`+"\n"), 0o600))
	refused := local.run(t, "runs", "--import="+empty)

	t.When("gotest runs --import names a saved stream of a run", func(w *gotest.T) {
		w.It("lists it among the records as imported, with its packages and the file's path", func(it *gotest.T) {
			gotest.Equal(it, 0, imported.Exit, "stderr: %s", imported.Stderr)
			gotest.Len(it, doc.Runs, 1)
			gotest.True(it, doc.Runs[0].Imported)
			gotest.Equal(it, saved, doc.Runs[0].EventsFile)
			gotest.Equal(it, 2, doc.Runs[0].Packages.Total)
		})
		w.It("refuses with exit 2 a file that holds no package verdict", func(it *gotest.T) {
			gotest.Equal(it, 2, refused.Exit, "stdout: %s", refused.Stdout)
			gotest.Contains(it, refused.Stderr, "the stream holds no package's verdict")
		})
	})
}
