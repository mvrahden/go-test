package testkit_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// RunOraclesTestSuite drives the real CLI over the markers fixture and checks
// what the oracles read back from it.
// Sequential: both methods share one built CLI and each spawns a full run.
type RunOraclesTestSuite struct {
	run testkit.Run
}

func (s *RunOraclesTestSuite) SuiteConfig() gotest.SuiteConfig {
	// No deadline: this suite compiles code, so its time is not a verdict.
	return gotest.SuiteConfig{Timeout: gotest.NoDeadline, SetupTimeout: gotest.NoDeadline}
}

func (s *RunOraclesTestSuite) BeforeAll(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "./testdata/markers/")
	gotest.NoError(t, err)
	s.run = run
}

func (s *RunOraclesTestSuite) AfterAll(t *gotest.T) {}

func (s *RunOraclesTestSuite) TestRunPackageReportsTheRun(t *gotest.T) {
	t.When("a package with a passing and a failing method ran", func(w *gotest.T) {
		w.It("exits 1", func(it *gotest.T) {
			gotest.Equal(it, 1, s.run.Exit)
		})
		w.It("reports each method's verdict", func(it *gotest.T) {
			gotest.Equal(it, "pass", s.run.Verdict("TestMarkerTestSuite/TestOne"))
			gotest.Equal(it, "fail", s.run.Verdict("TestMarkerTestSuite/TestTwo"))
			gotest.Equal(it, "fail", s.run.Verdict("TestMarkerTestSuite"))
		})
		w.It("reports the package verdict for the empty path", func(it *gotest.T) {
			gotest.Equal(it, "fail", s.run.Verdict(""))
		})
		w.It("reports no verdict for a test that never ran", func(it *gotest.T) {
			gotest.Zero(it, s.run.Verdict("TestMarkerTestSuite/TestNine"))
		})
		w.It("keeps the stream's events", func(it *gotest.T) {
			gotest.NotEmpty(it, s.run.Events)
			gotest.Contains(it, s.run.Stdout, `"Action":"fail"`)
		})
	})

	t.When("the suite stamped its lifecycle", func(w *gotest.T) {
		w.It("reads the markers in write order", func(it *gotest.T) {
			gotest.Equal(it, []string{"beforeAll", "TestOne", "afterAll"}, s.run.Markers)
		})
		w.It("confirms a relative order that holds", func(it *gotest.T) {
			gotest.True(it, s.run.MarkerOrder("beforeAll", "TestOne", "afterAll"))
			gotest.True(it, s.run.MarkerOrder("beforeAll", "afterAll"))
		})
		w.It("rejects an order that does not hold or a marker that is absent", func(it *gotest.T) {
			gotest.False(it, s.run.MarkerOrder("afterAll", "beforeAll"))
			gotest.False(it, s.run.MarkerOrder("beforeAll", "missing"))
		})
	})
}

func (s *RunOraclesTestSuite) TestRunCLIReportsStreamsAndExit(t *gotest.T) {
	run, err := testkit.RunCLI(t.Context(), ".", "version")
	gotest.NoError(t, err)
	t.When("a bare subcommand succeeds", func(w *gotest.T) {
		w.It("exits 0 with its output on stdout", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit)
			gotest.Regexp(it, "gotest", run.Stdout)
			gotest.Empty(it, run.Events)
		})
	})
	t.When("a subcommand is unknown", func(w *gotest.T) {
		bad, err := testkit.RunCLI(t.Context(), ".", "--no-such-flag")
		gotest.NoError(t, err)
		w.It("reports a non-zero exit with the complaint on stderr", func(it *gotest.T) {
			gotest.NotZero(it, bad.Exit)
			gotest.NotEmpty(it, bad.Stderr)
		})
	})
}

// RepoRootTestSuite: the repository is found by its go.mod however the
// checkout wrote the file's line endings.
type RepoRootTestSuite struct{}

func (s *RepoRootTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *RepoRootTestSuite) TestTheModuleLineIsRecognised(t *gotest.T) {
	t.When("go.mod names this module", func(w *gotest.T) {
		w.It("is recognised with LF line endings", func(it *gotest.T) {
			gotest.True(it, testkit.ExportIsRepoModule([]byte("module github.com/mvrahden/go-test\n\ngo 1.26.0\n")))
		})
		w.It("is recognised with CRLF line endings, as a Windows checkout writes them", func(it *gotest.T) {
			gotest.True(it, testkit.ExportIsRepoModule([]byte("module github.com/mvrahden/go-test\r\n\r\ngo 1.26.0\r\n")))
		})
		w.It("does not take a consumer module for the repository", func(it *gotest.T) {
			gotest.False(it, testkit.ExportIsRepoModule([]byte("module github.com/mvrahden/go-test-consumer\n")))
			gotest.False(it, testkit.ExportIsRepoModule([]byte("module example.com/m\n\nrequire github.com/mvrahden/go-test v1.0.0\n")))
		})
	})
}

// UserCacheTestSuite: a run started through the helpers keeps its gotest
// cache beside its markers, never in the user's cache. The CLI resolves that
// one as os.UserCacheDir()/gotest, so the run gets every variable it reads on
// any platform pointed into a fake home.
// Sequential: it moves the process's home for the run.
type UserCacheTestSuite struct {
	userCache string
	markers   string
	run       testkit.Run
}

func (s *UserCacheTestSuite) SuiteConfig() gotest.SuiteConfig {
	// No deadline: this suite compiles code, so its time is not a verdict.
	return gotest.SuiteConfig{SetupTimeout: gotest.NoDeadline}
}

func (s *UserCacheTestSuite) BeforeAll(t *gotest.T) {
	home := t.TempDir()
	s.markers = t.TempDir()
	// The go caches stay the real ones, so the fake home costs no rebuild.
	out, err := exec.CommandContext(t.Context(), "go", "env", "GOCACHE", "GOMODCACHE", "GOPATH", "GOENV").Output()
	gotest.NoError(t, err)
	goEnv := strings.Split(strings.TrimSpace(string(out)), "\n")
	gotest.Len(t, goEnv, 4)
	for i, key := range []string{"GOCACHE", "GOMODCACHE", "GOPATH", "GOENV"} {
		t.Setenv(key, goEnv[i])
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "xdg-cache"))
	t.Setenv("LocalAppData", filepath.Join(home, "AppData", "Local"))
	base, err := os.UserCacheDir()
	gotest.NoError(t, err)
	s.userCache = filepath.Join(base, "gotest")
	gotest.Regexp(t, "^"+regexp.QuoteMeta(home), s.userCache, "the user cache is not in the fake home")
	s.run, err = testkit.RunPackage(t.Context(), s.markers, "./testdata/markers/")
	gotest.NoError(t, err)
}

func (s *UserCacheTestSuite) AfterAll(t *gotest.T) {}

func (s *UserCacheTestSuite) TestTheUserCacheStaysUntouched(t *gotest.T) {
	t.When("a package ran through the helper", func(w *gotest.T) {
		w.It("ran it", func(it *gotest.T) {
			gotest.Equal(it, "pass", s.run.Verdict("TestMarkerTestSuite/TestOne"), s.run.Stderr)
		})
		w.It("leaves nothing in the user's gotest cache", func(it *gotest.T) {
			gotest.Empty(it, files(it, s.userCache))
		})
		w.It("keeps the test binaries in the run's own cache", func(it *gotest.T) {
			gotest.NotEmpty(it, files(it, filepath.Join(testkit.ChildCacheDir(s.markers), "bin")))
		})
	})
}

// files lists the regular files under dir; a missing dir has none.
func files(t *gotest.T, dir string) []string {
	var found []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) {
			return filepath.SkipDir
		}
		if err == nil && !d.IsDir() {
			found = append(found, path)
		}
		return err
	})
	gotest.NoError(t, err)
	return found
}
