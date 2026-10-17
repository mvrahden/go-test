package testkit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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

// RunPackage runs the built CLI with -json over pkg (a path relative to the
// caller's working directory) and reads back the stream and the markers the
// run left in markerDir. A non-zero exit is a result, not an error; only a
// run that could not start or be read is.
func RunPackage(ctx context.Context, markerDir, pkg string, args ...string) (Run, error) {
	return runCLI(ctx, ".", markerDir, nil, append([]string{"-json", pkg}, args...)...)
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
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	run := Run{}
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return Run{}, fmt.Errorf("run gotest %s: %w", strings.Join(args, " "), err)
		}
		run.Exit = exitErr.ExitCode()
	}
	run.Stdout, run.Stderr = stdout.String(), stderr.String()
	run.Events = parseEvents(stdout.Bytes())
	run.Markers, err = readMarkers(filepath.Join(markerDir, MarkersFile))
	return run, err
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
func (r *Run) MarkerOrder(markers ...string) bool {
	last := -1
	for _, m := range markers {
		idx := -1
		for i, got := range r.Markers {
			if got == m {
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
// directory, so a run under test sees only the flags it was given.
func childEnv(markerDir string) []string {
	env := make([]string, 0, len(os.Environ())+1)
	for _, kv := range os.Environ() {
		switch {
		case strings.HasPrefix(kv, "GITHUB_ACTIONS="),
			strings.HasPrefix(kv, "GITHUB_STEP_SUMMARY="),
			strings.HasPrefix(kv, "GOTEST_UPDATE_SNAPSHOTS="),
			strings.HasPrefix(kv, "GOTEST_CI="),
			strings.HasPrefix(kv, "CI="),
			strings.HasPrefix(kv, "GOTEST_CANARY_DIR="):
			continue
		}
		env = append(env, kv)
	}
	return append(env, "GOTEST_CANARY_DIR="+markerDir)
}

func parseEvents(stdout []byte) []Event {
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

func readMarkers(path string) ([]string, error) {
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
		dir, err := os.MkdirTemp("", "gotest-cli-")
		if err != nil {
			cliErr = err
			return
		}
		cliBinary, cliErr = BuildCLI(ctx, root, dir)
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
		if err == nil && strings.HasPrefix(string(data), "module github.com/mvrahden/go-test\n") {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("repository root not found above " + dir)
		}
		dir = parent
	}
}
