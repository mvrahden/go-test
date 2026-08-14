package main_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// FuzzTriagePromoteTestSuite exercises "gotest fuzz triage" and "gotest fuzz
// promote" end-to-end against the real examples/notification fixture. It
// runs sequentially (no SuiteConfig override — the default) because each
// test mutates shared, real repo files (a crasher fixture under
// testdata/fuzz/, and examples/notification/suite_test.go itself for
// promote) that BeforeEach/AfterEach create and restore per the suite
// lifecycle convention: no defer/Cleanup, resources are suite fields torn
// down in AfterEach.
//
//nolint:lifecycle-pair // BeforeAll's binary lives under t.TempDir(), which the framework removes automatically
type FuzzTriagePromoteTestSuite struct {
	binary           string
	repoRoot         string
	suiteTestPath    string
	fuzzRootDir      string // examples/notification/testdata/fuzz — entirely ours, safe to remove wholesale
	corpusFile       string
	structCorpusFile string

	origSuiteSrc []byte
}

func (s *FuzzTriagePromoteTestSuite) BeforeAll(t *gotest.T) {
	absRoot, err := filepath.Abs("../..")
	gotest.NoError(t, err)
	s.repoRoot = absRoot

	binDir := t.TempDir()
	binaryName := "gotest"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	s.binary = filepath.Join(binDir, binaryName)
	cmd := exec.Command("go", "build", "-o", s.binary, "./cmd/gotest") //nolint:gosec // G204: go tool with controlled arguments
	cmd.Dir = absRoot
	out, err := cmd.CombinedOutput()
	gotest.NoError(t, err, "build gotest binary: %s", string(out))

	s.suiteTestPath = filepath.Join(absRoot, "examples", "notification", "suite_test.go")
	s.fuzzRootDir = filepath.Join(absRoot, "examples", "notification", "testdata", "fuzz")
}

// BeforeEach snapshots suite_test.go (promote rewrites it in place) and
// plants two crasher fixtures under s.fuzzRootDir:
//
//   - a single stale crasher for FuzzTrim — a corpus entry whose recorded
//     input (`strings.TrimSpace("stale")` is already idempotent) no longer
//     fails FuzzTrim's property, so triage reports it as resolved rather
//     than as a real regression, and promote has exactly one well-known
//     seed to splice.
//   - a single crasher for FuzzSummary, the struct-typed target: its
//     corpus file is a native []byte entry (that's the real on-disk shape
//     for any struct-rerouted fuzz target), which triage must re-run
//     through the codec and report as a decoded Notification{...} literal
//     rather than the raw []byte(...) corpus text.
func (s *FuzzTriagePromoteTestSuite) BeforeEach(t *gotest.T) {
	orig, err := os.ReadFile(s.suiteTestPath)
	gotest.NoError(t, err)
	s.origSuiteSrc = orig

	dir := filepath.Join(s.fuzzRootDir, "FuzzNotificationServiceTestSuite_FuzzTrim")
	gotest.NoError(t, os.MkdirAll(dir, 0755))
	s.corpusFile = filepath.Join(dir, "stale-seed")
	gotest.NoError(t, os.WriteFile(s.corpusFile, []byte("go test fuzz v1\nstring(\"stale\")\n"), 0600))

	structDir := filepath.Join(s.fuzzRootDir, "FuzzNotificationServiceTestSuite_FuzzSummary")
	gotest.NoError(t, os.MkdirAll(structDir, 0755))
	s.structCorpusFile = filepath.Join(structDir, "struct-seed")
	gotest.NoError(t, os.WriteFile(s.structCorpusFile, []byte("go test fuzz v1\n[]byte(\"hello\")\n"), 0600))
}

func (s *FuzzTriagePromoteTestSuite) AfterEach(t *gotest.T) {
	gotest.NoError(t, os.WriteFile(s.suiteTestPath, s.origSuiteSrc, 0644)) //nolint:gosec // G306: restoring tracked source, not sensitive
	gotest.NoError(t, os.RemoveAll(s.fuzzRootDir))
}

func (s *FuzzTriagePromoteTestSuite) runCLIExit(t *gotest.T, args ...string) (string, int) {
	cmd := exec.Command(s.binary, args...) //nolint:gosec // G204: controlled binary with fixed args
	cmd.Dir = s.repoRoot
	out, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	gotest.True(t, err == nil || errors.As(err, &exitErr), "running gotest binary: %v\n%s", err, out)
	code := 0
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	return string(out), code
}

func (s *FuzzTriagePromoteTestSuite) TestTriage_StaleCrasherNoLongerFailing(t *gotest.T) {
	out, code := s.runCLIExit(t, "fuzz", "triage", "./examples/notification")

	t.It("reports the crasher and its decoded input", func(it *gotest.T) {
		gotest.Contains(it, out, "FuzzNotificationServiceTestSuite_FuzzTrim: 1 crasher")
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
	out, code := s.runCLIExit(t, "fuzz", "triage", "./examples/notification")

	t.It("reports the struct crasher and its decoded literal", func(it *gotest.T) {
		gotest.Contains(it, out, "FuzzNotificationServiceTestSuite_FuzzSummary: 1 crasher")
		gotest.Contains(it, out, "input: Notification{")
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
	bad := filepath.Join(s.fuzzRootDir, "FuzzNotificationServiceTestSuite_FuzzTrim", "garbled")
	gotest.NoError(t, os.WriteFile(bad, []byte("not a corpus file\n"), 0600))

	out, code := s.runCLIExit(t, "fuzz", "triage", "./examples/notification")

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
	out, code := s.runCLIExit(t, "fuzz", "--target=FuzzNoSuchTestSuite_FuzzNothing", "./examples/notification")

	t.It("exits 2 and lists the real targets", func(it *gotest.T) {
		gotest.Equal(it, 2, code)
		gotest.Contains(it, out, `no fuzz target named "FuzzNoSuchTestSuite_FuzzNothing"`)
		gotest.Contains(it, out, "FuzzNotificationServiceTestSuite_FuzzTrim")
	})
}

// TestSubcommandGrammar pins the strictly positional subcommand grammar: a
// misplaced or flag-preceded triage/promote is a loud usage error, never a
// silent reinterpretation — the historical readings either started a fuzz
// run the user did not ask for or dropped their flags on the floor.
func (s *FuzzTriagePromoteTestSuite) TestSubcommandGrammar(t *gotest.T) {
	t.When("the subcommand trails the package pattern", func(it *gotest.T) {
		out, code := s.runCLIExit(it, "fuzz", "./examples/notification", "triage")

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
		out, code := s.runCLIExit(it, "fuzz", "triage", "--for=5m", "./examples/notification")

		it.It("rejects it instead of silently ignoring it", func(it *gotest.T) {
			gotest.Equal(it, 2, code)
			gotest.Contains(it, out, "takes no flags")
		})
	})
}

func (s *FuzzTriagePromoteTestSuite) TestPromote_SplicesSeedAndDeletesCrasher(t *gotest.T) {
	out, code := s.runCLIExit(t, "fuzz", "promote", "./examples/notification")

	t.It("exits 0 and reports the promotion", func(it *gotest.T) {
		gotest.Equal(it, 0, code)
		gotest.Contains(it, out, "promoted FuzzNotificationServiceTestSuite_FuzzTrim/stale-seed")
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
	out, code := s.runCLIExit(t, "fuzz", "promote", "./examples/notification")

	t.It("exits 0 and reports the promotion", func(it *gotest.T) {
		gotest.Equal(it, 0, code)
		gotest.Contains(it, out, "promoted FuzzNotificationServiceTestSuite_FuzzSummary/struct-seed")
	})

	t.It("splices a typed Notification{...} literal into the suite source, not raw bytes", func(it *gotest.T) {
		rewritten, err := os.ReadFile(s.suiteTestPath)
		gotest.NoError(it, err)
		gotest.Contains(it, string(rewritten), "f.Add(Notification{")
		gotest.NotContains(it, string(rewritten), "f.Add([]byte(")
	})

	t.It("deletes the crasher file, since it's now a permanent seed", func(it *gotest.T) {
		_, err := os.Stat(s.structCorpusFile)
		gotest.True(it, os.IsNotExist(err))
	})
}
