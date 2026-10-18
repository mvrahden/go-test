package main_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	. "github.com/mvrahden/go-test/cmd/gotest"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// fakeSource stands in for the file watcher: the test feeds it events and
// reads back what the loop asked it to watch.
type fakeSource struct {
	events chan fsnotify.Event
	errs   chan error
	mu     sync.Mutex
	added  []string
}

func newFakeSource() *fakeSource {
	return &fakeSource{events: make(chan fsnotify.Event, 16), errs: make(chan error, 1)}
}
func (f *fakeSource) Events() <-chan fsnotify.Event { return f.events }
func (f *fakeSource) Errors() <-chan error          { return f.errs }
func (f *fakeSource) Add(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.added = append(f.added, path)
	return nil
}
func (f *fakeSource) addedPaths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.added...)
}

// WatchLoopTestSuite: the loop reruns what changed, follows directories that
// appear, and ends with the interrupt's exit code.
// Sequential: every method drives a loop on its own goroutine with timers.
type WatchLoopTestSuite struct{}

func (s *WatchLoopTestSuite) TestPatternsFromDirectories(t *gotest.T) {
	t.When("a directory becomes a package pattern", func(w *gotest.T) {
		for it, tc := range gotest.Each(w, []struct {
			Desc string
			dir  string
			want string
		}{
			{"a bare relative directory gets ./", "pkg/a", "./pkg/a"},
			{"an explicit relative directory stays", "./pkg/a", "./pkg/a"},
			{"a parent-relative directory stays", "../x/y", "../x/y"},
			{"the current directory stays", ".", "."},
			{"an absolute directory stays absolute", "/tmp/mod/pkg", "/tmp/mod/pkg"},
		}) {
			gotest.Equal(it, tc.want, ExportDirPattern(tc.dir))
		}
	})
}

func (s *WatchLoopTestSuite) TestChangedDirectoriesExpandToTheirDependents(t *gotest.T) {
	reverse := map[string][]string{"/m/core": {"/m/api", "/m/web"}, "/m/api": {"/m/web"}}
	t.When("a non-test dependency changed", func(w *gotest.T) {
		w.It("reruns it and every package that depends on it, once each, in order", func(it *gotest.T) {
			gotest.Equal(it, []string{"/m/api", "/m/core", "/m/web"}, ExportRerunDirs(map[string]bool{"/m/core": true}, reverse))
		})
	})
	t.When("a package with no dependents changed", func(w *gotest.T) {
		w.It("reruns just that package", func(it *gotest.T) {
			gotest.Equal(it, []string{"/m/web"}, ExportRerunDirs(map[string]bool{"/m/web": true}, reverse))
		})
	})
}

func (s *WatchLoopTestSuite) TestLoopFollowsTheFileSystem(t *gotest.T) {
	root := t.TempDir()
	src := newFakeSource()
	var mu sync.Mutex
	var runs [][]string
	run := func(dirs []string) {
		mu.Lock()
		defer mu.Unlock()
		runs = append(runs, dirs)
	}
	ctx, cancel := context.WithCancel(context.Background())
	code := make(chan int, 1)
	go func() {
		code <- ExportWatchLoop(ctx, src, 20*time.Millisecond, func() map[string][]string { return nil }, run)
	}()

	newDir := filepath.Join(root, "later")
	gotest.NoError(t, os.Mkdir(newDir, 0o750))
	src.events <- fsnotify.Event{Name: newDir, Op: fsnotify.Create}
	src.events <- fsnotify.Event{Name: filepath.Join(root, "a", "a_test.go"), Op: fsnotify.Remove}
	src.events <- fsnotify.Event{Name: filepath.Join(root, "a", "notes.txt"), Op: fsnotify.Write}

	gotest.Eventually(t, 2*time.Second, 10*time.Millisecond, func(poll *gotest.R) {
		mu.Lock()
		defer mu.Unlock()
		gotest.Len(poll, runs, 1)
	})
	cancel()
	exit := <-code

	t.When("a directory is created, a Go file removed and a text file written", func(w *gotest.T) {
		w.It("starts watching the new directory", func(it *gotest.T) {
			gotest.Contains(it, src.addedPaths(), newDir)
		})
		w.It("reruns the directory of the removed Go file and ignores the text file", func(it *gotest.T) {
			mu.Lock()
			defer mu.Unlock()
			gotest.Equal(it, [][]string{{filepath.Join(root, "a")}}, runs)
		})
	})
	t.When("the context is cancelled", func(w *gotest.T) {
		w.It("returns the interrupt's exit code", func(it *gotest.T) {
			gotest.Equal(it, 130, exit)
		})
	})
}
