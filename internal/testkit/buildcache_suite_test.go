package testkit_test

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// Sequential: each test chooses the optional build-cache environment.
type BuildCacheTestSuite struct{}

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
