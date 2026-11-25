package gotestrunner_test

import (
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// RunHistoryTestSuite: last.json and the history of one checkout, written
// by runs side by side.
type RunHistoryTestSuite struct{}

func (s *RunHistoryTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

// runOf is the record of a run of pkg alone in dir.
func runOf(dir, pkg, verdict string) protocol.RunRecord {
	rec := protocol.RunRecord{Dir: dir, Packages: []protocol.PackageVerdict{{Package: pkg, Verdict: verdict, InputHash: "h-" + pkg}}}
	if verdict == "fail" {
		rec.Failed = []protocol.FailedUnit{{Package: pkg, Test: "TestX/TestY"}}
	}
	return rec
}

func (s *RunHistoryTestSuite) TestMergingRuns(t *gotest.T) {
	t.When("--changed runs of different packages end side by side", func(w *gotest.T) {
		w.It("leaves last.json with every package's verdict and the history with every run", func(it *gotest.T) {
			cache := it.TempDir()
			const n = 24
			var wg sync.WaitGroup
			for i := range n {
				wg.Go(func() {
					gotest.NoError(it, gotestrunner.ExportSaveRunIn(cache, runOf("/work", fmt.Sprintf("p%02d", i), "pass"), true, time.Now()))
				})
			}
			wg.Wait()
			last, err := gotestrunner.ExportReadRecordIn(cache, "/work")
			gotest.NoError(it, err)
			var got []string
			for _, p := range last.Packages {
				got = append(got, p.Package)
			}
			slices.Sort(got)
			gotest.Len(it, got, n)
			gotest.Len(it, gotestrunner.ExportKeptRunsIn(cache, "/work"), n)
		})
	})
	t.When("a --changed run follows a run that failed another package", func(w *gotest.T) {
		w.It("keeps the other package's failed units, and drops those of a package it ran again", func(it *gotest.T) {
			cache := it.TempDir()
			now := time.Now()
			gotest.NoError(it, gotestrunner.ExportSaveRunIn(cache, runOf("/work", "a", "fail"), false, now))
			gotest.NoError(it, gotestrunner.ExportSaveRunIn(cache, runOf("/work", "b", "fail"), true, now.Add(time.Second)))
			gotest.NoError(it, gotestrunner.ExportSaveRunIn(cache, runOf("/work", "a", "pass"), true, now.Add(2*time.Second)))
			last, err := gotestrunner.ExportReadRecordIn(cache, "/work")
			gotest.NoError(it, err)
			gotest.Equal(it, []protocol.FailedUnit{{Package: "b", Test: "TestX/TestY"}}, last.Failed)
			gotest.Len(it, last.Packages, 2)
		})
	})
	t.When("a run without --changed follows", func(w *gotest.T) {
		w.It("replaces last.json and lists the newest kept run first", func(it *gotest.T) {
			cache := it.TempDir()
			now := time.Now()
			gotest.NoError(it, gotestrunner.ExportSaveRunIn(cache, runOf("/work", "a", "pass"), false, now))
			gotest.NoError(it, gotestrunner.ExportSaveRunIn(cache, runOf("/work", "b", "pass"), false, now.Add(time.Second)))
			last, err := gotestrunner.ExportReadRecordIn(cache, "/work")
			gotest.NoError(it, err)
			gotest.Equal(it, []protocol.PackageVerdict{{Package: "b", Verdict: "pass", InputHash: "h-b"}}, last.Packages)
			kept := gotestrunner.ExportKeptRunsIn(cache, "/work")
			gotest.Len(it, kept, 2)
			gotest.Equal(it, "b", kept[0].Record.Packages[0].Package)
			removed, inUse := gotestrunner.ExportClearRunsIn(cache)
			gotest.Equal(it, []int{1, 0}, []int{removed, inUse})
			gotest.Empty(it, gotestrunner.ExportKeptRunsIn(cache, "/work"))
		})
	})
}
