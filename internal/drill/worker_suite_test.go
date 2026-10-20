package drill_test

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"

	"github.com/mvrahden/go-test/internal/drill"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// WorkerTestSuite: a worker's verdict from synthetic events, not a real
// CLI, patch or Go invocation. Only grep actually runs, against the
// synthetic log.
type WorkerTestSuite struct{}

func (s *WorkerTestSuite) SuiteGuard() string {
	if runtime.GOOS != "linux" {
		return "the drill runs on Linux only"
	}
	return ""
}

func (s *WorkerTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type workerRun struct {
	verdict drill.Verdict
	kept    []string // the cache directories whose entries outlived the mutant
}

// work judges one synthetic mutant whose judge prints stream and exits with
// exitCode, and whose go build exits with buildCode.
func work(t *gotest.T, stream string, exitCode, buildCode int) workerRun {
	root := t.TempDir()
	snapshot := filepath.Join(root, "snapshot")
	gotest.NoError(t, os.Mkdir(snapshot, 0o755))
	patch := filepath.Join(root, "synthetic.patch")
	gotest.NoError(t, os.WriteFile(patch, []byte("Expect: a named failure\n--- a/unused\n"), 0o600))
	result := filepath.Join(root, "result.json")
	caches := []string{"children", "cli"}
	for _, d := range caches {
		dir := filepath.Join(root, "worker", "cache", d)
		gotest.NoError(t, os.MkdirAll(dir, 0o755))
		gotest.NoError(t, os.WriteFile(filepath.Join(dir, "entry"), []byte("from an earlier mutant"), 0o600))
	}
	spec := drill.Spec{Name: "synthetic", Work: filepath.Join(root, "worker"), Snapshot: snapshot, Patch: patch,
		Log: filepath.Join(root, "output.log"), Result: result,
		Runs: []drill.Run{{Pattern: "-", Packages: []string{"./synthetic"}}}, Judge: "synthetic-judge", CPUs: 1}
	run := func(cmd *exec.Cmd) (int, error) {
		switch {
		case cmd.Args[0] == "grep":
			return drill.RunCommand(cmd)
		case cmd.Args[0] == "go":
			return buildCode, nil
		case slices.Contains(cmd.Args, "-json"):
			_, err := io.WriteString(cmd.Stdout, stream)
			return exitCode, err
		case slices.Contains(cmd.Args, "summary"):
			_, err := io.WriteString(cmd.Stdout, "a named failure\n")
			return 0, err
		}
		return 0, nil
	}
	drill.Worker(&spec, run)
	var out workerRun
	for _, d := range caches {
		if _, err := os.Stat(filepath.Join(root, "worker", "cache", d, "entry")); err == nil {
			out.kept = append(out.kept, d)
		}
	}
	data, err := os.ReadFile(result)
	gotest.NoError(t, err)
	gotest.NoError(t, json.Unmarshal(data, &out.verdict))
	return out
}

func (s *WorkerTestSuite) TestAnAssertionFailureMatchesTheNamedCheck(t *gotest.T) {
	verdict := work(t, `{"Action":"fail","Test":"TestCheck"}`+"\n", 1, 0).verdict
	gotest.True(t, verdict.Caught, "%s", verdict.Reason)
	var phases []string
	for p := range verdict.Phases {
		phases = append(phases, p)
	}
	gotest.ElementsMatch(t, []string{"copy", "patch", "build", "judge", "render"}, phases)
}

func (s *WorkerTestSuite) TestOnlyTheGoValidatedCacheOutlivesAMutant(t *gotest.T) {
	gotest.Equal(t, []string{"cli"}, work(t, `{"Action":"fail","Test":"TestCheck"}`+"\n", 1, 0).kept)
}

func (s *WorkerTestSuite) TestABuildFailureIsNeverACatch(t *gotest.T) {
	verdict := work(t, `{"Action":"build-fail","ImportPath":"p"}`+"\n", 2, 0).verdict
	gotest.False(t, verdict.Caught)
	gotest.Contains(t, verdict.Reason, "harness does not compile")
	gotest.False(t, work(t, "", 1, 1).verdict.Caught)
}

func (s *WorkerTestSuite) TestASurvivorCannotPassThroughMatchingLogText(t *gotest.T) {
	gotest.False(t, work(t, `{"Action":"pass"}`+"\n", 0, 0).verdict.Caught)
}

func (s *WorkerTestSuite) TestEmptyMalformedAndAbnormalRunsFail(t *gotest.T) {
	entries := []struct {
		Desc   string
		stream string
		code   int
	}{
		{"no events", "", 2},
		{"not JSON", "not JSON", 1},
		{"not an event", "[]", 1},
		{"killed judge", `{"Action":"fail"}` + "\n", -9},
	}
	for t, entry := range gotest.Each(t, entries) {
		gotest.False(t, work(t, entry.stream, entry.code, 0).verdict.Caught)
	}
}

func (s *WorkerTestSuite) TestAWorkerEndedByASignalIsNamedByIt(t *gotest.T) {
	verdict := drill.ReadResult(filepath.Join(t.TempDir(), "result"), "a", &drill.Status{Exit: exit(-15)}, 0)
	gotest.Equal(t, "worker ended by SIGTERM", verdict.Reason)
}
