package cli_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIStaleBinaryTestSuite: whatever an earlier run left in the binary
// cache, a run executes the binary its sources, tags and link flags give.
// Each method stages the probe package and a cache of its own.
type CLIStaleBinaryTestSuite struct{}

func (s *CLIStaleBinaryTestSuite) SuiteConfig() gotest.SuiteConfig {
	// Up to four builds of a two-file package per method.
	return gotest.SuiteConfig{Parallel: true, Timeout: 60 * time.Second}
}

type staleCtx struct {
	dir, cache string
}

func (s *CLIStaleBinaryTestSuite) BeforeEach(t *gotest.T) *staleCtx {
	root := gotest.Must(testkit.RepoRoot())
	dir := gotest.Must(testkit.StagePackage(root, filepath.Join("..", "testdata", "staleprobe"), t.TempDir(), "staleprobe.example"))
	return &staleCtx{dir: dir, cache: t.TempDir()}
}

// cli runs the CLI on the probe package with the method's cache.
func (c *staleCtx) cli(t *gotest.T, env []string, args ...string) testkit.Run {
	env = append([]string{protocol.EnvCacheDir + "=" + c.cache}, env...)
	r, err := testkit.RunCLIWithEnv(t.Context(), c.dir, env, append([]string{".", "-v"}, args...)...)
	gotest.NoError(t, err)
	return r
}

var probeLine = regexp.MustCompile(`(?m)^stale-probe: .*$`)

// run is a passing run's probe line.
func (c *staleCtx) run(t *gotest.T, env []string, args ...string) string {
	r := c.cli(t, env, args...)
	gotest.Equal(t, 0, r.Exit, "stderr: %s", r.Stderr)
	line := probeLine.FindString(r.Stdout)
	gotest.NotEmpty(t, line)
	return line
}

// edit replaces old by replacement once in the probe's file.
func (c *staleCtx) edit(t *gotest.T, file, old, replacement string) {
	path := filepath.Join(c.dir, file)
	data := string(gotest.Must(os.ReadFile(path)))
	gotest.Contains(t, data, old)
	gotest.NoError(t, os.WriteFile(path, []byte(strings.Replace(data, old, replacement, 1)), 0o600))
}

func (s *CLIStaleBinaryTestSuite) TestADependencyChanges(t *gotest.T, c *staleCtx) {
	var lines []string
	lines = append(lines, c.run(t, nil))
	c.edit(t, "dep/dep.go", "return 1", "return 2")
	lines = append(lines, c.run(t, nil))
	c.edit(t, "dep/dep.go", "return 2", "return 1")
	lines = append(lines, c.run(t, nil))
	t.When("a dependency's source is changed, run, and restored", func(w *gotest.T) {
		w.It("each run prints the value its sources hold", func(it *gotest.T) {
			gotest.Equal(it, []string{
				"stale-probe: dep=1 test=1 tag=off linked=",
				"stale-probe: dep=2 test=1 tag=off linked=",
				"stale-probe: dep=1 test=1 tag=off linked=",
			}, lines)
		})
	})
}

func (s *CLIStaleBinaryTestSuite) TestATestFileChanges(t *gotest.T, c *staleCtx) {
	var lines []string
	lines = append(lines, c.run(t, nil))
	c.edit(t, "suite_test.go", "testValue = 1", "testValue = 2")
	lines = append(lines, c.run(t, nil))
	c.edit(t, "suite_test.go", "testValue = 2", "testValue = 1")
	lines = append(lines, c.run(t, nil))
	t.When("a test file is changed, run, and restored", func(w *gotest.T) {
		w.It("each run prints the value its sources hold", func(it *gotest.T) {
			gotest.Equal(it, []string{
				"stale-probe: dep=1 test=1 tag=off linked=",
				"stale-probe: dep=1 test=2 tag=off linked=",
				"stale-probe: dep=1 test=1 tag=off linked=",
			}, lines)
		})
	})
}

func (s *CLIStaleBinaryTestSuite) TestBuildTags(t *gotest.T, c *staleCtx) {
	lines := []string{
		c.run(t, nil),
		c.run(t, nil, "-tags", "probetag"),
		c.run(t, []string{"GOFLAGS=-tags=probetag"}),
		c.run(t, nil),
	}
	t.When("a run is given a build tag by -tags, then by GOFLAGS, then none", func(w *gotest.T) {
		w.It("each run executes the binary built for its tags", func(it *gotest.T) {
			gotest.Equal(it, []string{
				"stale-probe: dep=1 test=1 tag=off linked=",
				"stale-probe: dep=1 test=1 tag=on linked=",
				"stale-probe: dep=1 test=1 tag=on linked=",
				"stale-probe: dep=1 test=1 tag=off linked=",
			}, lines)
		})
	})
}

func (s *CLIStaleBinaryTestSuite) TestLinkFlags(t *gotest.T, c *staleCtx) {
	lines := []string{
		c.run(t, nil),
		c.run(t, nil, "-ldflags=-X=staleprobe.example/staleprobe.linked=arg"),
		c.run(t, []string{"GOFLAGS=-ldflags=-X=staleprobe.example/staleprobe.linked=env"}),
		c.run(t, nil),
	}
	t.When("a run is given -ldflags=-X as an argument, then in GOFLAGS, then none", func(w *gotest.T) {
		w.It("each run prints the value it was given", func(it *gotest.T) {
			gotest.Equal(it, []string{
				"stale-probe: dep=1 test=1 tag=off linked=",
				"stale-probe: dep=1 test=1 tag=off linked=arg",
				"stale-probe: dep=1 test=1 tag=off linked=env",
				"stale-probe: dep=1 test=1 tag=off linked=",
			}, lines)
		})
	})
}

func (s *CLIStaleBinaryTestSuite) TestVetAfterARunWithoutIt(t *gotest.T, c *staleCtx) {
	c.edit(t, "suite_test.go", "\tfmt.Printf(\"stale-probe:", "\tfmt.Printf(\"%d\\n\", \"vet\")\n\tfmt.Printf(\"stale-probe:")
	off := c.cli(t, nil, "-vet=off")
	on := c.cli(t, nil)
	t.When("a package with a vet finding is run with -vet=off and then without", func(w *gotest.T) {
		w.It("the second run reports the finding", func(it *gotest.T) {
			gotest.Equal(it, 0, off.Exit, "stderr: %s", off.Stderr)
			gotest.Equal(it, 2, on.Exit, "stdout: %s", on.Stdout)
			gotest.Contains(it, on.Stdout+on.Stderr, `has arg "vet" of wrong type string`)
		})
	})
}

func (s *CLIStaleBinaryTestSuite) TestRunsAtOnce(t *gotest.T, c *staleCtx) {
	runs := make([]testkit.Run, 3)
	errs := make([]error, 3)
	waits := make([]func(), 3)
	env := []string{protocol.EnvCacheDir + "=" + c.cache}
	for i := range runs {
		waits[i] = gotest.Go(t, func() {
			runs[i], errs[i] = testkit.RunCLIWithEnv(t.Context(), c.dir, env, ".")
		})
	}
	for _, wait := range waits {
		wait()
	}
	temps, err := filepath.Glob(filepath.Join(c.cache, "bin", "*", "*.tmp-*"))
	gotest.NoError(t, err)
	t.When("three runs of one package start at once on an empty cache", func(w *gotest.T) {
		w.It("all pass and leave no temporary file in the cache", func(it *gotest.T) {
			for i := range runs {
				gotest.NoError(it, errs[i])
				gotest.Equal(it, 0, runs[i].Exit, "stderr: %s", runs[i].Stderr)
			}
			gotest.Empty(it, temps)
		})
	})
}

func (s *CLIStaleBinaryTestSuite) TestRunsAtOnceWithGOFLAGSTags(t *gotest.T, c *staleCtx) {
	gotest.Equal(t, "stale-probe: dep=1 test=1 tag=off linked=", c.run(t, nil))
	// Every run's go touches the cached binary its slot links when it finds
	// it up to date; touch it all along, as a third run would, so the two
	// runs below see such a touch whenever they look.
	stop := make(chan struct{})
	waitTouch := gotest.Go(t, func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(10 * time.Millisecond):
			}
			bins, _ := filepath.Glob(filepath.Join(c.cache, "bin", "*", "*.test"))
			now := time.Now()
			for _, b := range bins {
				_ = os.Chtimes(b, now, now)
			}
		}
	})
	var tagged, plain string
	waitTagged := gotest.Go(t, func() { tagged = c.run(t, []string{"GOFLAGS=-tags=probetag"}) })
	waitPlain := gotest.Go(t, func() { plain = c.run(t, nil) })
	waitTagged()
	waitPlain()
	close(stop)
	waitTouch()
	t.When("two runs of one package start together over one cache, one with a build tag in GOFLAGS", func(w *gotest.T) {
		w.It("each executes the binary built for its own tags", func(it *gotest.T) {
			gotest.Equal(it, "stale-probe: dep=1 test=1 tag=on linked=", tagged)
			gotest.Equal(it, "stale-probe: dep=1 test=1 tag=off linked=", plain)
		})
	})
}

func (s *CLIStaleBinaryTestSuite) TestAFilteredRunBeforeAFullOne(t *gotest.T, c *staleCtx) {
	env := []string{protocol.EnvCacheDir + "=" + c.cache}
	filtered, err := testkit.RunCLIWithEnv(t.Context(), ".", env, "../testdata/buildset/...", "-run", "^TestSelTestSuite$")
	gotest.NoError(t, err)
	full, err := testkit.RunCLIWithEnv(t.Context(), ".", env, "../testdata/buildset/...")
	gotest.NoError(t, err)
	t.When("a run that -run narrows to one package is followed by an unfiltered run", func(w *gotest.T) {
		w.It("the second run builds and runs the other package", func(it *gotest.T) {
			gotest.Equal(it, 0, filtered.Exit, "stderr: %s", filtered.Stderr)
			gotest.Equal(it, 2, full.Exit)
			gotest.Contains(it, full.Stdout+full.Stderr, `has arg "not a number" of wrong type string`)
		})
	})
}
