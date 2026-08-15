package gotestrunner_test

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FuzzPreflightTestSuite covers the stale-corpus pre-flight: a corpus entry
// recorded before the fuzzed type changed shape still parses, so nothing but
// this comparison can tell the user why their next run dies on a type error
// naming a generated wrapper.
type FuzzPreflightTestSuite struct{ dir string }

func (s *FuzzPreflightTestSuite) BeforeEach(t *gotest.T) {
	dir, err := os.MkdirTemp("", "gotest-preflight-*")
	gotest.NoError(t, err)
	s.dir = dir
}

func (s *FuzzPreflightTestSuite) AfterEach(_ *gotest.T) {
	if s.dir != "" {
		os.RemoveAll(s.dir)
	}
}

// writeCorpus writes one corpus entry for funcName, with the given
// already-rendered "Type(value)" lines.
func (s *FuzzPreflightTestSuite) writeCorpus(t *gotest.T, funcName, name string, lines ...string) {
	dir := filepath.Join(s.dir, "testdata", "fuzz", funcName)
	gotest.NoError(t, os.MkdirAll(dir, 0o755))
	body := "go test fuzz v1\n"
	for _, l := range lines {
		body += l + "\n"
	}
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
}

func (s *FuzzPreflightTestSuite) TestCheckFuzzCorpus(t *gotest.T) {
	t.When("every entry matches the target's current shape", func(w *gotest.T) {
		w.It("reports no mismatch", func(it *gotest.T) {
			s.writeCorpus(it, "FuzzT_Match", "aaa", `string("hi")`, `[]byte("\x01\x02")`)

			got, err := gotestrunner.CheckFuzzCorpus(s.dir, "FuzzT_Match", []string{"string", "[]byte"})

			gotest.NoError(it, err)
			gotest.Empty(it, got)
		})
	})

	t.When("an entry holds fewer values than the target takes", func(w *gotest.T) {
		w.It("reports the count drift with both shapes", func(it *gotest.T) {
			s.writeCorpus(it, "FuzzT_Count", "bbb", `string("hi")`)

			got, err := gotestrunner.CheckFuzzCorpus(s.dir, "FuzzT_Count", []string{"string", "[]byte"})

			gotest.NoError(it, err)
			gotest.Len(it, got, 1)
			gotest.Equal(it, []string{"string"}, got[0].Got)
			gotest.Equal(it, []string{"string", "[]byte"}, got[0].Want)
			gotest.Equal(it, "testdata/fuzz/FuzzT_Count/bbb", got[0].File)
		})
	})

	t.When("an entry holds the right count but the wrong types", func(w *gotest.T) {
		w.It("reports the type drift — a same-arity field swap reinterprets the leaves", func(it *gotest.T) {
			s.writeCorpus(it, "FuzzT_Types", "ccc", `[]byte("\x01")`, `string("hi")`)

			got, err := gotestrunner.CheckFuzzCorpus(s.dir, "FuzzT_Types", []string{"string", "[]byte"})

			gotest.NoError(it, err)
			gotest.Len(it, got, 1)
			gotest.Equal(it, []string{"[]byte", "string"}, got[0].Got)
		})
	})

	t.When("an entry cannot be parsed", func(w *gotest.T) {
		w.It("skips it — an unreadable entry is triage's business, not shape drift", func(it *gotest.T) {
			s.writeCorpus(it, "FuzzT_Unparsed", "ddd", `Request{Kind: 1}`)

			got, err := gotestrunner.CheckFuzzCorpus(s.dir, "FuzzT_Unparsed", []string{"string"})

			gotest.NoError(it, err)
			gotest.Empty(it, got)
		})
	})

	t.When("the target has no corpus directory", func(w *gotest.T) {
		w.It("reports nothing and no error", func(it *gotest.T) {
			got, err := gotestrunner.CheckFuzzCorpus(s.dir, "FuzzT_Missing", []string{"string"})

			gotest.NoError(it, err)
			gotest.Empty(it, got)
		})
	})
}

func (s *FuzzPreflightTestSuite) TestCorpusMismatchMessage(t *gotest.T) {
	t.It("names the entry, both shapes, and both ways out", func(it *gotest.T) {
		m := gotestrunner.CorpusMismatch{
			Func: "FuzzT_A",
			File: "testdata/fuzz/FuzzT_A/bbb",
			Got:  []string{"string"},
			Want: []string{"string", "[]byte"},
		}

		gotest.Equal(it, "fuzz: FuzzT_A: testdata/fuzz/FuzzT_A/bbb has 1 values of [string], but the target now takes 2 [string, []byte] — it predates a change to the fuzzed type's fields; run gotest fuzz promote to turn it into a typed f.Add seed, or delete it", m.Message())
	})
}

func (s *FuzzPreflightTestSuite) TestReportStaleFuzzCorpora(t *gotest.T) {
	t.When("the overlay records a target whose corpus drifted", func(w *gotest.T) {
		w.It("writes one warning per stale entry", func(it *gotest.T) {
			s.writeCorpus(it, "FuzzT_A", "bbb", `string("hi")`)
			s.writeCorpus(it, "FuzzT_B", "ccc", `string("hi")`)
			overlay := &gotestrunner.OverlayResult{
				DirsByPkg: map[string]string{"example.com/p": s.dir},
				FuzzParamsByFunc: map[string]map[string][]string{"example.com/p": {
					"FuzzT_A": {"string", "[]byte"},
					"FuzzT_B": {"string"},
				}},
			}

			var buf bytes.Buffer
			gotestrunner.ReportStaleFuzzCorpora(&buf, overlay)

			gotest.Contains(it, buf.String(), "fuzz: FuzzT_A: testdata/fuzz/FuzzT_A/bbb has 1 values of [string]")
			gotest.NotContains(it, buf.String(), "FuzzT_B")
		})
	})

	t.When("the session runs a subset of the targets", func(w *gotest.T) {
		w.It("checks only those, so --target never reports another target's drift", func(it *gotest.T) {
			s.writeCorpus(it, "FuzzT_A", "bbb", `string("hi")`)
			s.writeCorpus(it, "FuzzT_B", "ccc", `string("hi")`)
			overlay := &gotestrunner.OverlayResult{
				DirsByPkg: map[string]string{"example.com/p": s.dir},
				FuzzParamsByFunc: map[string]map[string][]string{"example.com/p": {
					"FuzzT_A": {"string", "[]byte"},
					"FuzzT_B": {"bool"},
				}},
			}

			var buf bytes.Buffer
			gotestrunner.ReportStaleFuzzCorporaFor(&buf, overlay, []gotestrunner.FuzzTarget{
				{Package: "example.com/p", Dir: s.dir, Func: "FuzzT_B"},
			})

			gotest.Contains(it, buf.String(), "fuzz: FuzzT_B:")
			gotest.NotContains(it, buf.String(), "FuzzT_A")
		})
	})
}
