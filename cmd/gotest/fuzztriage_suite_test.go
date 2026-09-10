package main_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// FuzzTriagePromoteTestSuite exercises "gotest fuzz triage" and "gotest fuzz
// promote" end-to-end on the staged fuzzcrash module, with crasher files
// planted the way the engine writes them. Each test gets its own copy of
// the module under t.TempDir(), so promote's source edit and the corpus
// files never touch the repository and nothing needs restoring.
//
//nolint:lifecycle-pair // BeforeAll's binary lives under t.TempDir(), which the framework removes automatically
type FuzzTriagePromoteTestSuite struct {
	binary           string
	repoRoot         string
	pkgDir           string
	suiteTestPath    string
	corpusFile       string
	structCorpusFile string
}

func (s *FuzzTriagePromoteTestSuite) BeforeAll(t *gotest.T) {
	absRoot, err := filepath.Abs("../..")
	gotest.NoError(t, err)
	s.repoRoot = absRoot
	s.binary = buildGotestBinary(t, absRoot, t.TempDir())
}

// BeforeEach stages the module and plants two crasher fixtures:
//
//   - a stale crasher for FuzzTrim — a corpus entry whose recorded input
//     (`strings.TrimSpace("stale")` is already idempotent) no longer fails
//     the property, so triage reports it as resolved rather than as a real
//     regression, and promote has exactly one well-known seed to splice.
//   - a crasher for FuzzSummary, the struct-typed target: its corpus file
//     holds one native value per leaf of Message's fan (three strings and
//     the []byte carrying Priority — the real on-disk shape for any fanned
//     target), which triage must re-run and report as the decoded
//     Message{...} literal rather than as the raw per-leaf corpus text.
func (s *FuzzTriagePromoteTestSuite) BeforeEach(t *gotest.T) {
	s.pkgDir = stageFuzzModule(t, s.repoRoot, t.TempDir())
	s.suiteTestPath = filepath.Join(s.pkgDir, "suite_test.go")
	fuzzRoot := filepath.Join(s.pkgDir, "testdata", "fuzz")

	dir := filepath.Join(fuzzRoot, "FuzzMessageTestSuite_FuzzTrim")
	gotest.NoError(t, os.MkdirAll(dir, 0o750))
	s.corpusFile = filepath.Join(dir, "stale-seed")
	gotest.NoError(t, os.WriteFile(s.corpusFile, []byte("go test fuzz v1\nstring(\"stale\")\n"), 0o600))

	structDir := filepath.Join(fuzzRoot, "FuzzMessageTestSuite_FuzzSummary")
	gotest.NoError(t, os.MkdirAll(structDir, 0o750))
	s.structCorpusFile = filepath.Join(structDir, "struct-seed")
	gotest.NoError(t, os.WriteFile(s.structCorpusFile, []byte("go test fuzz v1\nstring(\"a@b.c\")\nstring(\"welcome\")\nstring(\"\")\n[]byte(\"\\x02\")\n"), 0o600))
}

func (s *FuzzTriagePromoteTestSuite) runCLIExit(t *gotest.T, args ...string) (string, int) {
	return runGotestIn(t, s.binary, s.pkgDir, nil, args...)
}

func (s *FuzzTriagePromoteTestSuite) TestTriage_StaleCrasherNoLongerFailing(t *gotest.T) {
	out, code := s.runCLIExit(t, "fuzz", "triage", ".")

	t.It("reports the crasher and its decoded input", func(it *gotest.T) {
		gotest.Contains(it, out, "FuzzMessageTestSuite_FuzzTrim: 1 crasher")
		gotest.Contains(it, out, `input: string("stale")`)
	})
	t.It("re-runs it and finds it no longer fails", func(it *gotest.T) {
		gotest.Contains(it, out, "status: no longer failing")
	})
	t.It("exits 0", func(it *gotest.T) {
		gotest.Equal(it, 0, code)
	})
}

func (s *FuzzTriagePromoteTestSuite) TestTriage_StructCrasherShowsDecodedInput(t *gotest.T) {
	out, code := s.runCLIExit(t, "fuzz", "triage", ".")

	t.It("reports the struct crasher and its decoded literal", func(it *gotest.T) {
		gotest.Contains(it, out, "FuzzMessageTestSuite_FuzzSummary: 1 crasher")
		gotest.Contains(it, out, "input: Message{")
	})
	t.It("does not fall back to the raw []byte corpus display", func(it *gotest.T) {
		gotest.NotContains(it, out, "[]byte(")
	})
	t.It("exits 0", func(it *gotest.T) {
		gotest.Equal(it, 0, code)
	})
}

// TestTriage_UnparseableCrasherFails pins the exit contract shared with
// promote: a crasher file that cannot even be read is a failure, not a
// silent skip — a directory of unreadable crashers must not "pass" triage.
func (s *FuzzTriagePromoteTestSuite) TestTriage_UnparseableCrasherFails(t *gotest.T) {
	bad := filepath.Join(s.pkgDir, "testdata", "fuzz", "FuzzMessageTestSuite_FuzzTrim", "garbled")
	gotest.NoError(t, os.WriteFile(bad, []byte("not a corpus file\n"), 0600))

	out, code := s.runCLIExit(t, "fuzz", "triage", ".")

	t.It("reports the unreadable crasher", func(it *gotest.T) {
		gotest.Contains(it, out, "garbled")
	})
	t.It("exits 1 like promote does", func(it *gotest.T) {
		gotest.Equal(it, 1, code)
	})
}

// TestFuzzTargetFlag_UnknownName pins the --target contract end-to-end: a
// name that matches no generated wrapper is a usage error listing the
// available targets, never a silent fall-through to fuzzing everything.
func (s *FuzzTriagePromoteTestSuite) TestFuzzTargetFlag_UnknownName(t *gotest.T) {
	out, code := s.runCLIExit(t, "fuzz", "--target=FuzzNoSuchTestSuite_FuzzNothing", ".")

	t.It("exits 2 and lists the real targets", func(it *gotest.T) {
		gotest.Equal(it, 2, code)
		gotest.Contains(it, out, `no fuzz target named "FuzzNoSuchTestSuite_FuzzNothing"`)
		gotest.Contains(it, out, "FuzzMessageTestSuite_FuzzTrim")
	})
}

// TestSubcommandGrammar pins the strictly positional subcommand grammar: a
// misplaced or flag-preceded triage/promote is a loud usage error, never a
// silent reinterpretation — the historical readings either started a fuzz
// run the user did not ask for or dropped their flags on the floor.
func (s *FuzzTriagePromoteTestSuite) TestSubcommandGrammar(t *gotest.T) {
	t.When("the subcommand trails the package pattern", func(it *gotest.T) {
		out, code := s.runCLIExit(it, "fuzz", ".", "triage")

		it.It("rejects it instead of starting a fuzz run", func(it *gotest.T) {
			gotest.Equal(it, 2, code)
			gotest.Contains(it, out, "must come immediately after fuzz")
		})
	})

	t.When("a flag precedes the subcommand", func(it *gotest.T) {
		out, code := s.runCLIExit(it, "fuzz", "--for=5m", "promote")

		it.It("rejects it instead of dropping the flag", func(it *gotest.T) {
			gotest.Equal(it, 2, code)
			gotest.Contains(it, out, "must come immediately after fuzz")
		})
	})

	t.When("a flag is passed to a subcommand", func(it *gotest.T) {
		out, code := s.runCLIExit(it, "fuzz", "triage", "--for=5m", ".")

		it.It("rejects it instead of silently ignoring it", func(it *gotest.T) {
			gotest.Equal(it, 2, code)
			gotest.Contains(it, out, "takes no flags")
		})
	})
}

func (s *FuzzTriagePromoteTestSuite) TestPromote_SplicesSeedAndDeletesCrasher(t *gotest.T) {
	out, code := s.runCLIExit(t, "fuzz", "promote", ".")

	t.It("exits 0 and reports the promotion", func(it *gotest.T) {
		gotest.Equal(it, 0, code)
		gotest.Contains(it, out, "promoted FuzzMessageTestSuite_FuzzTrim/stale-seed")
		gotest.Contains(it, out, `f.Add("stale")`)
	})

	t.It("splices f.Add(\"stale\") into the suite source", func(it *gotest.T) {
		got, err := os.ReadFile(s.suiteTestPath)
		gotest.NoError(it, err)
		gotest.Contains(it, string(got), `f.Add("stale")`)
	})

	t.It("deletes the crasher file, since it's now a permanent seed", func(it *gotest.T) {
		_, err := os.Stat(s.corpusFile)
		gotest.True(it, os.IsNotExist(err))
	})
}

func (s *FuzzTriagePromoteTestSuite) TestPromote_SplicesStructSeedAsTypedLiteral(t *gotest.T) {
	out, code := s.runCLIExit(t, "fuzz", "promote", ".")

	t.It("exits 0 and reports the promotion", func(it *gotest.T) {
		gotest.Equal(it, 0, code)
		gotest.Contains(it, out, "promoted FuzzMessageTestSuite_FuzzSummary/struct-seed")
	})

	t.It("splices a typed Message{...} literal into the suite source, not raw bytes", func(it *gotest.T) {
		rewritten, err := os.ReadFile(s.suiteTestPath)
		gotest.NoError(it, err)
		gotest.Contains(it, string(rewritten), "f.Add(Message{")
		gotest.NotContains(it, string(rewritten), "f.Add([]byte(")
	})

	t.It("deletes the crasher file, since it's now a permanent seed", func(it *gotest.T) {
		_, err := os.Stat(s.structCorpusFile)
		gotest.True(it, os.IsNotExist(err))
	})
}
