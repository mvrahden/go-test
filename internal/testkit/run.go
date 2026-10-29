package testkit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/internal/protocol"
)

// Run is what one CLI invocation left behind: its exit code, both streams,
// the go test -json events found on stdout and the markers the suites under
// test stamped into the markers file.
type Run struct {
	Exit    int
	Stdout  string
	Stderr  string
	Events  []Event
	Markers []string
}

// Event is one go test -json line.
type Event struct {
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
	Output  string `json:"Output"`
}

// MarkersFile is the file, inside the directory GOTEST_CANARY_DIR names, that
// suites under test append one marker per line to.
const MarkersFile = "markers"

// StopGrace is how long a CLI whose context ended has to finish stopping
// before it is killed: the default fixture Timeout its teardown is given, and
// the 30s the process adds. It also bounds how long its output may be held
// open once it has exited. WithStopGrace sets another.
const StopGrace = 2*time.Minute + 30*time.Second

type stopGraceKey struct{}

// WithStopGrace returns ctx carrying grace in place of StopGrace for the runs
// started under it.
func WithStopGrace(ctx context.Context, grace time.Duration) context.Context {
	return context.WithValue(ctx, stopGraceKey{}, grace)
}

func stopGrace(ctx context.Context) time.Duration {
	if grace, ok := ctx.Value(stopGraceKey{}).(time.Duration); ok {
		return grace
	}
	return StopGrace
}

// RunPackage runs the built CLI with -json over pkg (a path relative to the
// caller's working directory) and reads back the stream and the markers the
// run left in markerDir. A non-zero exit is a result, not an error; only a
// run that could not start or be read is.
func RunPackage(ctx context.Context, markerDir, pkg string, args ...string) (Run, error) {
	return runCLI(ctx, ".", markerDir, nil, append([]string{"-json", pkg}, args...)...)
}

// RunPackageWithEnv is RunPackage with extra KEY=VALUE pairs appended to the
// child's environment after the scrub, so a fixture package under test can be
// told how to behave. A GOTEST_CACHE_DIR among them replaces the run's own.
func RunPackageWithEnv(ctx context.Context, markerDir string, env []string, pkg string, args ...string) (Run, error) {
	return runCLI(ctx, ".", markerDir, env, append([]string{"-json", pkg}, args...)...)
}

// RunCLI runs the built CLI in dir with args and returns its exit code and
// streams; -json events on stdout are parsed when present. Markers go to a
// temporary directory that is discarded, so Markers is empty.
func RunCLI(ctx context.Context, dir string, args ...string) (Run, error) {
	return runCLI(ctx, dir, "", nil, args...)
}

// RunCLIWithEnv is RunCLI with extra KEY=VALUE pairs appended to the child's
// environment after the scrub, so a check can put a scrubbed variable back
// on purpose.
func RunCLIWithEnv(ctx context.Context, dir string, env []string, args ...string) (Run, error) {
	return runCLI(ctx, dir, "", env, args...)
}

func runCLI(ctx context.Context, dir, markerDir string, extraEnv []string, args ...string) (Run, error) {
	binary, err := cli(ctx)
	if err != nil {
		return Run{}, err
	}
	return runBinary(ctx, binary, stopGrace(ctx), dir, markerDir, extraEnv, args...)
}

// runBinary is runCLI with binary as the CLI, given grace to stop.
func runBinary(ctx context.Context, binary string, grace time.Duration, dir, markerDir string, extraEnv []string, args ...string) (Run, error) {
	var err error
	if markerDir == "" {
		markerDir, err = os.MkdirTemp("", "gotest-markers-")
		if err != nil {
			return Run{}, err
		}
		defer os.RemoveAll(markerDir)
	}
	cmd := exec.CommandContext(ctx, binary, args...) //nolint:gosec // G204: the repo's own CLI with the caller's arguments
	cmd.Dir = dir
	cmd.Env = append(childEnv(markerDir), extraEnv...)
	// The pipes are the run's own, so a hold past the exit is seen whatever
	// the exit status: exec reports an *ExitError over ErrWaitDelay.
	var stdout, stderr bytes.Buffer
	outR, outW, err := os.Pipe()
	if err != nil {
		return Run{}, err
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		_, _ = outR.Close(), outW.Close()
		return Run{}, err
	}
	cmd.Stdout, cmd.Stderr = outW, errW
	var copying sync.WaitGroup
	copying.Add(2)
	go func() { defer copying.Done(); _, _ = io.Copy(&stdout, outR) }()
	go func() { defer copying.Done(); _, _ = io.Copy(&stderr, errR) }()
	copied := make(chan struct{})
	// A context that ends stops the CLI as a user does, so its shared
	// fixtures are torn down; it is killed only when it outlives the grace.
	tree := proctree.New(cmd)
	cmd.Cancel = func() error {
		if err := Interrupt(cmd, tree); err != nil {
			return os.ErrProcessDone
		}
		return nil
	}
	cmd.WaitDelay = grace
	run := Run{}
	var held error
	err = tree.Start()
	started := err == nil
	_, _ = outW.Close(), errW.Close()
	if started {
		err = cmd.Wait()
	}
	go func() { copying.Wait(); close(copied) }()
	select {
	case <-copied:
	case <-time.After(grace):
		// The CLI exited; a process it left behind holds its output.
		held = fmt.Errorf("run gotest %s: its output was still held open %s after it exited", strings.Join(args, " "), grace)
	}
	// Released only once the hold was judged: the release kills what the CLI left.
	if started {
		tree.Release()
	}
	_, _ = outR.Close(), errR.Close()
	<-copied
	if err != nil {
		var exitErr *exec.ExitError
		switch {
		case errors.As(err, &exitErr):
			run.Exit = exitErr.ExitCode()
		case cmd.ProcessState != nil:
			// Stopped by the context.
			run.Exit = cmd.ProcessState.ExitCode()
		default:
			return Run{}, fmt.Errorf("run gotest %s: %w", strings.Join(args, " "), err)
		}
	}
	run.Stdout, run.Stderr = stdout.String(), stderr.String()
	run.Events = ParseEvents(stdout.Bytes())
	run.Markers, err = ReadMarkers(filepath.Join(markerDir, MarkersFile))
	return run, errors.Join(held, err)
}

// Verdict is the last pass, fail or skip the stream recorded for the test
// path; "" or "-" names the package event. A test the stream never closed has
// no verdict and yields "".
func (r *Run) Verdict(path string) string {
	if path == "-" {
		path = ""
	}
	verdict := ""
	for _, ev := range r.Events {
		if ev.Test != path {
			continue
		}
		switch ev.Action {
		case "pass", "fail", "skip":
			verdict = ev.Action
		}
	}
	return verdict
}

// MarkerOrder reports whether the markers appear in the run in the order
// given, judged by each marker's first occurrence; an absent marker is false.
func (r *Run) MarkerOrder(markers ...string) bool { return MarkerOrderOf(r.Markers, markers...) }

// MarkerOrderOf is MarkerOrder over markers read some other way.
func MarkerOrderOf(got []string, markers ...string) bool {
	last := -1
	for _, m := range markers {
		idx := -1
		for i, g := range got {
			if g == m {
				idx = i
				break
			}
		}
		if idx < 0 || idx <= last {
			return false
		}
		last = idx
	}
	return true
}

// childEnv is the CLI's environment: the caller's, minus the GitHub Actions
// variables, the CI switches and the snapshot update switch, plus the marker
// directory, so a run under test sees only the flags it was given. Its gotest
// cache uses the drill's private worker root when supplied, otherwise the
// marker directory; neither fills the user's cache.
func childEnv(markerDir string) []string {
	env := make([]string, 0, len(os.Environ())+2)
	for _, kv := range os.Environ() {
		switch {
		case strings.HasPrefix(kv, "GITHUB_ACTIONS="),
			strings.HasPrefix(kv, "GITHUB_STEP_SUMMARY="),
			strings.HasPrefix(kv, "GITHUB_WORKSPACE="),
			strings.HasPrefix(kv, "GOTEST_UPDATE_SNAPSHOTS="),
			strings.HasPrefix(kv, "GOTEST_CI="),
			strings.HasPrefix(kv, "CI="),
			strings.HasPrefix(kv, "GOTEST_CANARY_DIR="),
			strings.HasPrefix(kv, protocol.EnvCacheDir+"="):
			continue
		}
		env = append(env, kv)
	}
	return append(env, "GOTEST_CANARY_DIR="+markerDir, protocol.EnvCacheDir+"="+ChildCacheDir(markerDir))
}

// ChildEnv is the environment runCLI gives the CLI, for a check that starts
// the CLI itself: the scrub, the marker directory and the cache inside it,
// then extra pairs, which may name another cache.
func ChildEnv(markerDir string, extra ...string) []string {
	return append(childEnv(markerDir), extra...)
}

// ParseEvents reads the go test -json events among stdout's lines.
func ParseEvents(stdout []byte) []Event {
	var events []Event
	for _, line := range bytes.Split(stdout, []byte("\n")) {
		if !bytes.HasPrefix(line, []byte("{")) {
			continue
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		events = append(events, ev)
	}
	return events
}

// ReadMarkers reads the markers file at path; a missing file is no markers.
func ReadMarkers(path string) ([]string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: a file inside the caller's own marker directory
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	text := strings.TrimRight(string(data), "\n")
	if text == "" {
		return nil, nil
	}
	return strings.Split(text, "\n"), nil
}

var (
	cliOnce   sync.Once
	cliBinary string
	cliErr    error
)

// cli builds the repository's CLI once per process and returns its path.
func cli(ctx context.Context) (string, error) {
	cliOnce.Do(func() {
		root, err := RepoRoot()
		if err != nil {
			cliErr = err
			return
		}
		cliBinary, cliErr = resolveCLI(ctx, root)
	})
	return cliBinary, cliErr
}

// RepoRoot walks up from the working directory to the go.mod that declares
// this repository's module.
func RepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil && isRepoModule(data) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("repository root not found above " + dir)
		}
		dir = parent
	}
}

// isRepoModule reports whether a go.mod declares this repository's module. A
// Windows checkout writes CRLF line endings, so the first line is compared
// without its terminator.
func isRepoModule(goMod []byte) bool {
	first, _, _ := strings.Cut(string(goMod), "\n")
	return strings.TrimRight(first, "\r") == "module github.com/mvrahden/go-test"
}
