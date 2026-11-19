package perf_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// tools builds the go shim and the exec counter into dir and returns the
// shim's directory, to put first in PATH, and the counter's path.
func tools(ctx context.Context, repoRoot, dir string) (shimDir, execCount string, err error) {
	shimDir = filepath.Join(dir, "shim")
	execCount = filepath.Join(dir, "execcount"+exeSuffix())
	builds := [][2]string{
		{filepath.Join(shimDir, "go"+exeSuffix()), "./tests/perf/cmd/goshim"},
		{execCount, "./tests/perf/cmd/execcount"},
	}
	for _, b := range builds {
		cmd := exec.CommandContext(ctx, "go", "build", "-o", b[0], b[1]) //nolint:gosec // G204: go with controlled arguments
		cmd.Dir = repoRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", "", errors.New("build " + b[1] + ": " + err.Error() + "\n" + string(out))
		}
	}
	return shimDir, execCount, nil
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// measured is one command's footprint.
type measured struct {
	Exit   int
	Stdout []byte
	Stderr []byte
	Go     [][]string    // argv of each go invocation, in start order
	Execs  int           // test binaries started through -exec
	CPU    time.Duration // user+system of the process tree; zero on Windows
}

// harness runs commands with the go shim first in PATH and reads back what
// the shim and the exec counter logged.
type harness struct {
	shimDir string
	realGo  string
	goLog   string
	execLog string
}

// measure runs name with args in dir, with the shim first in PATH. "go" is
// the shim itself. env is appended to the base environment.
func (h *harness) measure(ctx context.Context, dir, name string, env []string, args ...string) (measured, error) {
	for _, log := range []string{h.goLog, h.execLog} {
		if err := os.WriteFile(log, nil, 0o600); err != nil {
			return measured{}, err
		}
	}
	if name == "go" {
		name = filepath.Join(h.shimDir, "go"+exeSuffix())
	}
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // G204: the CLI under test or go
	cmd.Dir = dir
	cmd.Env = h.environ(env)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		return measured{}, err
	}
	m := measured{Exit: cmd.ProcessState.ExitCode(), Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	// On Unix the rusage of a reaped child includes the descendants it reaped.
	m.CPU = cmd.ProcessState.UserTime() + cmd.ProcessState.SystemTime()
	if m.Go, err = readGoLog(h.goLog); err != nil {
		return m, err
	}
	execs, err := os.ReadFile(h.execLog)
	if err != nil {
		return m, err
	}
	m.Execs = bytes.Count(execs, []byte("\n"))
	return m, nil
}

// environ is env, or this process's environment when env is nil, with the
// shim's variables and the shim first in PATH; a later duplicate key wins.
func (h *harness) environ(env []string) []string {
	if env == nil {
		env = os.Environ()
	}
	return append(slices.Clip(env),
		"GOTEST_PERF_GOLOG="+h.goLog,
		"GOTEST_PERF_EXECLOG="+h.execLog,
		"GOTEST_PERF_REAL_GO="+h.realGo,
		"PATH="+h.shimDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
}

// readGoLog splits the shim's log into one argv per line.
func readGoLog(path string) ([][]string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: the harness's own log
	if err != nil {
		return nil, err
	}
	var calls [][]string
	for line := range strings.SplitSeq(strings.TrimSuffix(string(data), "\n"), "\n") {
		if line != "" {
			calls = append(calls, strings.Split(line, "\x1f"))
		}
	}
	return calls, nil
}

// staging is what every perf suite shares: the staged modules at a fixed
// path, so Go's build cache sees the same directories on every run, the
// harness, the CLI and the environment its runs get.
type staging struct {
	h         *harness
	root      string
	m12, m6   string
	cli       string
	execCount string
	env       []string
}

// stageRoot is <user cache>/gotest/perf/<12 hex of the repository root's hash>.
func stageRoot(repoRoot string) (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(repoRoot))
	return filepath.Join(cache, "gotest", "perf", hex.EncodeToString(sum[:])[:12]), nil
}

func (st *staging) setUp(t *gotest.T) {
	repoRoot, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	st.root, err = stageRoot(repoRoot)
	gotest.NoError(t, err)
	gotest.NoError(t, os.RemoveAll(st.root))
	st.m12, st.m6 = filepath.Join(st.root, "m12"), filepath.Join(st.root, "m6")
	gotest.NoError(t, stage(repoRoot, st.m12, 12, 2, 3))
	gotest.NoError(t, stage(repoRoot, st.m6, 6, 2, 3))
	realGo, err := exec.LookPath("go")
	gotest.NoError(t, err)
	shimDir, execCount, err := tools(t.Context(), repoRoot, filepath.Join(st.root, "tools"))
	gotest.NoError(t, err)
	st.execCount = execCount
	st.h = &harness{shimDir: shimDir, realGo: realGo, goLog: filepath.Join(st.root, "go.log"), execLog: filepath.Join(st.root, "exec.log")}
	st.cli, err = testkit.BuildCLI(t.Context(), repoRoot, st.root)
	gotest.NoError(t, err)
	st.env = testkit.ChildEnv(st.root, protocol.EnvCacheDir+"="+filepath.Join(st.root, "cache"))
}

func (st *staging) tearDown(t *gotest.T) {
	if st.root != "" {
		gotest.NoError(t, os.RemoveAll(st.root))
	}
}
