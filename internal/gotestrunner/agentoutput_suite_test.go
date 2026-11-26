package gotestrunner_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AgentOutputTestSuite covers what a run says to a coding agent: its closing
// line, the failures it renders from the stream, and the record --failed
// reruns.
type AgentOutputTestSuite struct{}

func (s *AgentOutputTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func intp(n int) *int { return &n }

func (s *AgentOutputTestSuite) TestTheClosingLine(t *gotest.T) {
	for t, r := range gotest.Each(t, []struct {
		Desc    string
		closing gotestrunner.Closing
		want    string
	}{
		{
			Desc:    "a red run counted test by test, with the rerun command",
			closing: gotestrunner.Closing{ExitCode: 1, Tests: &protocol.Tally{Passed: 2, Failed: 2}, NotRun: intp(0), Packages: protocol.Tally{Passed: 1, Failed: 1}, Rerun: "gotest --failed"},
			want:    "gotest: FAIL exit 1 | tests: 2 passed, 2 failed, 0 skipped, 0 not run | packages: 1 passed, 1 failed, 0 skipped | rerun: gotest --failed",
		},
		{
			Desc:    "a green text run, which counts packages",
			closing: gotestrunner.Closing{Packages: protocol.Tally{Passed: 3, Skipped: 1}, NotRun: intp(0)},
			want:    "gotest: PASS exit 0 | packages: 3 passed, 0 failed, 1 skipped",
		},
		{
			Desc:    "a text run the census failed",
			closing: gotestrunner.Closing{ExitCode: 2, Packages: protocol.Tally{Failed: 1}, NotRun: intp(3)},
			want:    "gotest: FAIL exit 2 | tests: 3 not run | packages: 0 passed, 1 failed, 0 skipped",
		},
		{
			Desc:    "a green run in which nothing passed",
			closing: gotestrunner.Closing{Tests: &protocol.Tally{Skipped: 2}, Packages: protocol.Tally{Passed: 1}},
			want:    "gotest: EMPTY exit 0 | tests: 0 passed, 0 failed, 2 skipped | packages: 1 passed, 0 failed, 0 skipped",
		},
	}) {
		t.It("reads as the run judged it", func(it *gotest.T) {
			gotest.Equal(it, r.want, r.closing.String())
		})
	}

	t.It("counts the units the census booked as not run instead of failed", func(it *gotest.T) {
		res := &protocol.RunResult{Tests: protocol.Tally{Failed: 3}, NotRun: intp(2), Packages: protocol.Tally{Failed: 1}}
		gotest.Equal(it, &protocol.Tally{Failed: 1}, gotestrunner.ClosingOf(res, 2, true).Tests)
		gotest.Zero(it, gotestrunner.ClosingOf(res, 2, false).Tests)
	})
}

// agentStream is a -json run of a passing and a failing package, closed.
const agentStream = `{"Action":"start","Package":"gotest run","RunLevel":true,"Manifest":{"Version":"v1","Args":[]}}
{"Action":"run","Package":"example.com/ok","Test":"TestOkTestSuite"}
{"Action":"output","Package":"example.com/ok","Test":"TestOkTestSuite","Output":"=== RUN   TestOkTestSuite\n"}
{"Action":"output","Package":"example.com/ok","Test":"TestOkTestSuite","Output":"    ok_test.go:3: a log of a passing test\n"}
{"Action":"pass","Package":"example.com/ok","Test":"TestOkTestSuite"}
{"Action":"output","Package":"example.com/ok","Output":"ok  \texample.com/ok\t0.010s\n"}
{"Action":"pass","Package":"example.com/ok"}
{"Action":"run","Package":"example.com/bad","Test":"TestBadTestSuite"}
{"Action":"output","Package":"example.com/bad","Test":"TestBadTestSuite","Output":"=== RUN   TestBadTestSuite\n"}
{"Action":"run","Package":"example.com/bad","Test":"TestBadTestSuite/TestGood"}
{"Action":"output","Package":"example.com/bad","Test":"TestBadTestSuite/TestGood","Output":"    good_test.go:5: a log of a passing method\n"}
{"Action":"output","Package":"example.com/bad","Test":"TestBadTestSuite/TestGood","Output":"    --- PASS: TestBadTestSuite/TestGood (0.00s)\n"}
{"Action":"pass","Package":"example.com/bad","Test":"TestBadTestSuite/TestGood"}
{"Action":"run","Package":"example.com/bad","Test":"TestBadTestSuite/TestBroken"}
{"Action":"output","Package":"example.com/bad","Test":"TestBadTestSuite/TestBroken","Output":"    bad_test.go:9: deliberate\n"}
{"Action":"output","Package":"example.com/bad","Test":"TestBadTestSuite/TestBroken","Output":"    --- FAIL: TestBadTestSuite/TestBroken (0.00s)\n"}
{"Action":"fail","Package":"example.com/bad","Test":"TestBadTestSuite/TestBroken"}
{"Action":"output","Package":"example.com/bad","Test":"TestBadTestSuite","Output":"--- FAIL: TestBadTestSuite (0.00s)\n"}
{"Action":"fail","Package":"example.com/bad","Test":"TestBadTestSuite"}
{"Action":"output","Package":"example.com/bad","Output":"FAIL\n"}
{"Action":"output","Package":"example.com/bad","Output":"FAIL\texample.com/bad\t0.020s\n"}
{"Action":"fail","Package":"example.com/bad"}
{"Action":"fail","Package":"gotest run","RunLevel":true,"Result":{"ExitCode":1,"Packages":{"Passed":1,"Failed":1,"Skipped":0},"Tests":{"Passed":2,"Failed":2,"Skipped":0}}}
`

func (s *AgentOutputTestSuite) TestRenderingTheStream(t *gotest.T) {
	var out bytes.Buffer
	a := gotestrunner.NewAgentOutput(&out)
	a.SetRerun("gotest --failed")
	// Written in two pieces, the cut inside a line.
	_, _ = a.Write([]byte(agentStream[:700]))
	_, _ = a.Write([]byte(agentStream[700:]))

	t.It("prints only the failed tests' output and their package's FAIL line, then the closing line", func(it *gotest.T) {
		gotest.Equal(it, "    bad_test.go:9: deliberate\n"+
			"    --- FAIL: TestBadTestSuite/TestBroken (0.00s)\n"+
			"--- FAIL: TestBadTestSuite (0.00s)\n"+
			"FAIL\texample.com/bad\t0.020s\n"+
			"gotest: FAIL exit 1 | tests: 2 passed, 2 failed, 0 skipped | packages: 1 passed, 1 failed, 0 skipped | rerun: gotest --failed\n", out.String())
		gotest.True(it, a.Closed())
	})

	t.It("names a test the census booked, and its package, though neither printed a result line", func(it *gotest.T) {
		var booked bytes.Buffer
		b := gotestrunner.NewAgentOutput(&booked)
		_, _ = b.Write([]byte(`{"Action":"run","Package":"example.com/p","Test":"TestPTestSuite/TestOne"}
{"Action":"output","Package":"example.com/p","Test":"TestPTestSuite/TestOne","Output":"    census: declared test never ran\n"}
{"Action":"fail","Package":"example.com/p","Test":"TestPTestSuite/TestOne"}
{"Action":"fail","Package":"example.com/p"}
`))
		gotest.Equal(it, "    census: declared test never ran\n--- FAIL: TestPTestSuite/TestOne\nFAIL\texample.com/p\n", booked.String())
		gotest.False(it, b.Closed())
	})

	t.It("prints a build's output as it comes", func(it *gotest.T) {
		var built bytes.Buffer
		b := gotestrunner.NewAgentOutput(&built)
		_, _ = b.Write([]byte(`{"ImportPath":"example.com/p [example.com/p.test]","Action":"build-output","Output":"# example.com/p\n"}
{"ImportPath":"example.com/p [example.com/p.test]","Action":"build-output","Output":"./p_test.go:3:1: syntax error\n"}
`))
		gotest.Equal(it, "# example.com/p\n./p_test.go:3:1: syntax error\n", built.String())
	})
}

func (s *AgentOutputTestSuite) TestTheUnitsARunFailed(t *gotest.T) {
	t.It("keeps a failed method for its suite, a fuzz wrapper for its seed, and a package that failed with no test", func(it *gotest.T) {
		judged, failed := gotestrunner.ExportStreamFailures(`{"Action":"fail","Package":"example.com/bad","Test":"TestBadTestSuite/TestBroken/when_it_breaks"}
{"Action":"fail","Package":"example.com/bad","Test":"TestBadTestSuite/TestBroken"}
{"Action":"fail","Package":"example.com/bad","Test":"TestBadTestSuite"}
{"Action":"fail","Package":"example.com/bad","Test":"FuzzBadTestSuite_FuzzIt/seed#0"}
{"Action":"fail","Package":"example.com/bad","Test":"TestHookTestSuite"}
{"Action":"fail","Package":"example.com/bad"}
{"Action":"fail","Package":"example.com/unbuilt","FailedBuild":"example.com/unbuilt [example.com/unbuilt.test]"}
`)
		gotest.Equal(it, []protocol.FailedUnit{
			{Package: "example.com/bad", Test: "TestBadTestSuite/TestBroken"},
			{Package: "example.com/bad", Test: "FuzzBadTestSuite_FuzzIt"},
			{Package: "example.com/bad", Test: "TestHookTestSuite"},
			{Package: "example.com/unbuilt"},
		}, failed)
		gotest.Equal(it, protocol.Tally{Failed: 2}, judged.Packages)
	})

	t.It("reads a text run's result lines for the methods that failed", func(it *gotest.T) {
		_, _, _, failed := gotestrunner.ExportTextCensus([]gotestrunner.ExportPackageResult{
			{Pkg: "example.com/bad", Result: gotestrunner.SuiteResult{
				Target:   gotestrunner.SuiteTarget{SuiteSpec: gotestrunner.SuiteSpec{SuiteName: "TestBadTestSuite"}},
				Stdout:   []byte("--- FAIL: TestBadTestSuite (0.00s)\n    --- FAIL: TestBadTestSuite/TestBroken (0.00s)\n        --- FAIL: TestBadTestSuite/TestBroken/when_it_breaks (0.00s)\nFAIL\n"),
				ExitCode: 1,
			}},
			{Pkg: "example.com/hook", Result: gotestrunner.SuiteResult{
				Target:   gotestrunner.SuiteTarget{SuiteSpec: gotestrunner.SuiteSpec{SuiteName: "TestHookTestSuite"}},
				Stdout:   []byte("--- FAIL: TestHookTestSuite (0.00s)\nFAIL\n"),
				ExitCode: 1,
			}},
			{Pkg: "example.com/unbuilt", Result: gotestrunner.SuiteResult{ExitCode: 2}},
		}, gotestrunner.DeclaredUnits{}, gotestrunner.PipelineConfig{})
		gotest.Equal(it, []protocol.FailedUnit{
			{Package: "example.com/bad", Test: "TestBadTestSuite/TestBroken"},
			{Package: "example.com/hook", Test: "TestHookTestSuite"},
			{Package: "example.com/unbuilt"},
		}, failed)
	})
}

func (s *AgentOutputTestSuite) TestRerunningTheFailures(t *gotest.T) {
	failed := []protocol.FailedUnit{
		{Package: "example.com/bad", Test: "TestBadTestSuite/TestBroken"},
		{Package: "example.com/bad", Test: "FuzzBadTestSuite_FuzzIt"},
		{Package: "example.com/unbuilt"},
	}
	only := gotestrunner.RerunSelection(failed)

	t.It("gives each package its own -run, and a package that failed whole none", func(it *gotest.T) {
		gotest.Equal(it, map[string]string{
			"example.com/bad":     "^TestBadTestSuite$/^TestBroken$|^FuzzBadTestSuite_FuzzIt$",
			"example.com/unbuilt": "",
		}, only)
	})

	t.It("holds the census to the units the selection reruns", func(it *gotest.T) {
		declared := []gotestrunner.CensusCase{
			{Pkg: "example.com/bad", Path: "TestBadTestSuite/TestBroken"},
			{Pkg: "example.com/bad", Path: "TestBadTestSuite/TestFine"},
			{Pkg: "example.com/bad", Path: "FuzzBadTestSuite_FuzzIt"},
			{Pkg: "example.com/unbuilt", Path: "TestUnbuiltTestSuite/TestAny"},
			{Pkg: "example.com/other", Path: "TestBadTestSuite/TestBroken"},
		}
		gotest.Equal(it, []gotestrunner.CensusCase{declared[0], declared[2], declared[3]}, gotestrunner.ExportRerunUnits(declared, only))
	})
}

func (s *AgentOutputTestSuite) TestTheRecord(t *gotest.T) {
	cache := t.TempDir()
	rec := protocol.RunRecord{Dir: "/work/module", Manifest: protocol.Manifest{Version: "v1", Args: []string{"./..."}}, Result: protocol.RunResult{ExitCode: 1}, Failed: []protocol.FailedUnit{{Package: "example.com/bad", Test: "TestBadTestSuite/TestBroken"}}}
	gotest.NoError(t, gotestrunner.ExportWriteRecordIn(cache, rec))

	t.It("lies in the cache under runs, keyed by the directory", func(it *gotest.T) {
		path := gotestrunner.ExportRecordPathIn(cache, rec.Dir)
		gotest.Regexp(it, `^runs/[0-9a-f]{16}/last\.json$`, filepath.ToSlash(strings.TrimPrefix(path, cache+string(filepath.Separator))))
		_, err := os.Stat(path)
		gotest.NoError(it, err)
		gotest.NotEqual(it, path, gotestrunner.ExportRecordPathIn(cache, "/work/other"))
	})

	t.It("reads back as it was written, and is not another directory's", func(it *gotest.T) {
		got, err := gotestrunner.ExportReadRecordIn(cache, rec.Dir)
		gotest.NoError(it, err)
		gotest.Equal(it, rec, got)
		_, err = gotestrunner.ExportReadRecordIn(cache, "/work/other")
		gotest.ErrorIs(it, err, os.ErrNotExist)
	})
}
