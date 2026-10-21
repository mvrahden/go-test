package testkit_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// Sequential: each test chooses the optional build-cache environment.
type BuildCacheTestSuite struct{}

func (s *BuildCacheTestSuite) SuiteConfig() gotest.SuiteConfig {
	// No deadline: this suite compiles code, so its time is not a verdict.
	return gotest.SuiteConfig{Timeout: gotest.NoDeadline}
}

func (s *BuildCacheTestSuite) TestCachedBuildsFollowSourcesAndKeepPrivateExecutables(t *gotest.T) {
	t.Setenv("GOTEST_TEST_CLI_CACHE_DIR", filepath.Join(t.TempDir(), "cache with spaces"))
	root := t.TempDir()
	mainDir := filepath.Join(root, "cmd", "gotest")
	gotest.NoError(t, os.MkdirAll(mainDir, 0o750))
	write(t, filepath.Join(root, "go.mod"), "module cachetest\n\ngo 1.26.0\n")
	write(t, filepath.Join(mainDir, "main.go"), "package main\nimport _ \"embed\"\n//go:embed note.txt\nvar note string\nfunc main(){print(note)}\n")
	note := filepath.Join(mainDir, "note.txt")
	write(t, note, "one")
	first, err := testkit.BuildCLI(t.Context(), root, t.TempDir())
	gotest.NoError(t, err)
	write(t, note, "two")
	second, err := testkit.BuildCLI(t.Context(), root, t.TempDir())
	gotest.NoError(t, err)
	for sub, row := range gotest.Each(t, []struct{ Name, Binary, Want string }{
		{"the first caller keeps its original executable", first, "one"},
		{"changed embedded sources produce another executable", second, "two"},
	}) {
		out, err := exec.CommandContext(sub.Context(), row.Binary).CombinedOutput() //nolint:gosec // G204: the executable this test just built
		gotest.NoError(sub, err)
		gotest.Equal(sub, row.Want, string(out))
	}
	write(t, filepath.Join(mainDir, "main.go"), "package main\nthis does not compile\n")
	_, err = testkit.BuildCLI(t.Context(), root, t.TempDir())
	gotest.Error(t, err, "a failed build must not return the previous cached executable")
}

func (s *BuildCacheTestSuite) TestChildBuildCachesAreSeparateFromMarkers(t *gotest.T) {
	t.Setenv("GOTEST_TEST_CACHE_DIR", "")
	gotest.Equal(t, filepath.Join("markers", "cache"), testkit.ChildCacheDir("markers"))
	t.Setenv("GOTEST_TEST_CACHE_DIR", "private-worker-cache")
	gotest.Equal(t, "private-worker-cache", testkit.ChildCacheDir("first-markers"))
	gotest.Equal(t, "private-worker-cache", testkit.ChildCacheDir("second-markers"))
}

// Sequential: it swaps how the process keys and builds the CLI.
type InProcessCacheTestSuite struct {
	full            []byte
	builds          atomic.Int32
	active, overlap atomic.Int32
	restore         func()
}

func (s *InProcessCacheTestSuite) SuiteConfig() gotest.SuiteConfig {
	// No deadline: this suite compiles code, so its time is not a verdict.
	return gotest.SuiteConfig{Timeout: gotest.NoDeadline}
}

func (s *InProcessCacheTestSuite) BeforeEach(t *gotest.T) {
	s.full = bytes.Repeat([]byte("x"), 64)
	s.builds.Store(0)
	s.active.Store(0)
	s.overlap.Store(0)
	s.restore = testkit.ExportFakeBuilds(func(context.Context, string) (string, error) { return "fixed", nil }, s.slowBuild)
}

func (s *InProcessCacheTestSuite) AfterEach(t *gotest.T) { s.restore() }

// slowBuild writes the output a byte at a time and counts overlapping builds.
func (s *InProcessCacheTestSuite) slowBuild(_ context.Context, _, dir string) (string, error) {
	s.builds.Add(1)
	if s.active.Add(1) > 1 {
		s.overlap.Add(1)
	}
	defer s.active.Add(-1)
	binary := filepath.Join(dir, testkit.ExportCLIName())
	f, err := os.Create(binary) //nolint:gosec // G304: the test's own cache dir
	if err != nil {
		return "", err
	}
	defer f.Close()
	for i := range s.full {
		if _, err := f.Write(s.full[i : i+1]); err != nil {
			return "", err
		}
		time.Sleep(time.Millisecond)
	}
	return binary, nil
}

func (s *InProcessCacheTestSuite) TestConcurrentCallersShareOneBuild(t *gotest.T) {
	t.Setenv("GOTEST_TEST_CLI_CACHE_DIR", t.TempDir())
	root := t.TempDir()
	var mu sync.Mutex
	var copies [][]byte
	var errs []error
	waits := make([]func(), 4)
	for i := range waits {
		dir := t.TempDir()
		waits[i] = gotest.Go(t, func() {
			binary, err := testkit.BuildCLI(t.Context(), root, dir)
			var data []byte
			if err == nil {
				data, err = os.ReadFile(binary) //nolint:gosec // G304: the private copy BuildCLI returned
			}
			mu.Lock()
			defer mu.Unlock()
			errs = append(errs, err)
			copies = append(copies, data)
		})
	}
	for _, wait := range waits {
		wait()
	}
	for _, err := range errs {
		gotest.NoError(t, err)
	}
	gotest.Equal(t, int32(1), s.builds.Load(), "callers of the same sources built more than once")
	gotest.Zero(t, s.overlap.Load(), "a build started while another was writing the same output")
	for _, data := range copies {
		gotest.Equal(t, s.full, data, "a caller copied a half-written output")
	}
}
