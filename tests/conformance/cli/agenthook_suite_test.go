package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIAgentHookTestSuite: gotest hook answers a coding agent's hook events
// without ever blocking the agent.
//
//nolint:lifecycle-pair // BeforeAll stages modules into temp dirs the test removes
type CLIAgentHookTestSuite struct {
	module, toolModule, plain string
}

func (s *CLIAgentHookTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: time.Minute}
}

func (s *CLIAgentHookTestSuite) BeforeAll(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	s.module = t.TempDir()
	gotest.NoError(t, testkit.StageModule(root, filepath.Join("..", "testdata", "cliagenthook"), s.module))
	s.toolModule = t.TempDir()
	gotest.NoError(t, testkit.StageModule(root, filepath.Join("..", "testdata", "cliagenthook"), s.toolModule))
	goMod := filepath.Join(s.toolModule, "go.mod")
	data, err := os.ReadFile(goMod) //nolint:gosec // G304: the module this suite staged
	gotest.NoError(t, err)
	gotest.NoError(t, os.WriteFile(goMod, append(data, "\ntool github.com/mvrahden/go-test/cmd/gotest\n"...), 0o600))
	s.plain = t.TempDir()
	gotest.NoError(t, testkit.StageModule(root, filepath.Join("..", "testdata", "cliagenthookplain"), s.plain))
}

// hookEvent is a hook event as an agent writes it on stdin.
func hookEvent(t *gotest.T, event, cwd string, toolInput map[string]string) string {
	data, err := json.Marshal(map[string]any{"hook_event_name": event, "cwd": cwd, "tool_input": toolInput})
	gotest.NoError(t, err)
	return string(data)
}

// hookAnswer is what gotest hook wrote on stdout, read back as JSON.
type hookAnswer struct {
	HookSpecificOutput map[string]string `json:"hookSpecificOutput"`
}

func (s *CLIAgentHookTestSuite) TestEvents(t *gotest.T) {
	t.When("gotest hook runs an event it does not know", func(w *gotest.T) {
		run, err := testkit.RunCLIWithInput(w.Context(), s.module, "{}", "hook", "after-everything")
		gotest.NoError(w, err)
		w.It("exits 1 and names the events it takes on stderr", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit)
			gotest.Empty(it, run.Stdout)
			gotest.Contains(it, run.Stderr, `"after-everything"`)
			gotest.Contains(it, run.Stderr, "post-edit, pre-shell or stop")
		})
	})
	t.When("gotest hook runs the stop event", func(w *gotest.T) {
		run, err := testkit.RunCLIWithInput(w.Context(), s.module, hookEvent(w, "Stop", s.module, nil), "hook", "stop")
		gotest.NoError(w, err)
		w.It("exits 0 and writes nothing", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit)
			gotest.Empty(it, run.Stdout)
			gotest.Empty(it, run.Stderr)
		})
	})
}

func (s *CLIAgentHookTestSuite) TestAfterAnEdit(t *gotest.T) {
	t.When("an agent edits a test file whose package has a lint finding, in a module that requires gotest", func(w *gotest.T) {
		event := hookEvent(w, "PostToolUse", s.module, map[string]string{"file_path": "finding/finding_test.go"})
		run, err := testkit.RunCLIWithInput(w.Context(), s.module, event, "hook", "post-edit")
		gotest.NoError(w, err)
		w.It("exits 0 with the finding as additional context and no permission decision", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stderr: %s", run.Stderr)
			gotest.Empty(it, run.Stderr)
			var answer hookAnswer
			gotest.NoError(it, json.Unmarshal([]byte(run.Stdout), &answer), "stdout: %s", run.Stdout)
			gotest.Equal(it, "PostToolUse", answer.HookSpecificOutput["hookEventName"])
			gotest.Contains(it, answer.HookSpecificOutput["additionalContext"], "finding_test.go:9:2: use Equal instead of True for == comparison (assertion-simplify)")
			gotest.NotContains(it, answer.HookSpecificOutput, "permissionDecision")
		})
	})

	t.When("an agent edits a clean test file, a file that is not a test, or a test file in a module without gotest", func(w *gotest.T) {
		w.It("exits 0 and writes nothing", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []struct {
				Desc   string
				module func() string
				file   string
			}{
				{Desc: "a clean test file", module: func() string { return s.module }, file: "clean/clean_test.go"},
				{Desc: "a file that is not a test", module: func() string { return s.module }, file: "finding/finding.go"},
				{Desc: "a test file in a module without gotest", module: func() string { return s.plain }, file: "suites/suites_test.go"},
			}) {
				dir := row.module()
				event := hookEvent(sub, "PostToolUse", dir, map[string]string{"file_path": filepath.Join(dir, row.file)})
				run, err := testkit.RunCLIWithInput(sub.Context(), dir, event, "hook", "post-edit")
				gotest.NoError(sub, err)
				gotest.Equal(sub, 0, run.Exit, "stderr: %s", run.Stderr)
				gotest.Empty(sub, run.Stdout)
				gotest.Empty(sub, run.Stderr)
			}
		})
	})
}

func (s *CLIAgentHookTestSuite) TestBeforeAShellCommand(t *gotest.T) {
	t.When("an agent is about to run go test, after an environment assignment or a cd, over packages that declare gotest suites", func(w *gotest.T) {
		w.It("exits 0 with a note giving the command line with gotest in place of go test, and no permission decision", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []struct {
				Desc    string
				tool    bool
				command string
				want    string
			}{
				{Desc: "every package", command: "go test ./...", want: "gotest ./..."},
				{Desc: "an environment assignment and flags", command: "CGO_ENABLED=0 go test -run TestAdds -v ./finding/", want: "CGO_ENABLED=0 gotest -run TestAdds -v ./finding/"},
				{Desc: "a cd before it", command: "cd finding && go test -count=1 | tail -5", want: "cd finding && gotest -count=1 | tail -5"},
				{Desc: "a module that declares the tool", tool: true, command: "go test ./...", want: "go tool gotest ./..."},
			}) {
				dir := s.module
				if row.tool {
					dir = s.toolModule
				}
				event := hookEvent(sub, "PreToolUse", dir, map[string]string{"command": row.command})
				run, err := testkit.RunCLIWithInput(sub.Context(), dir, event, "hook", "pre-shell")
				gotest.NoError(sub, err)
				gotest.Equal(sub, 0, run.Exit, "stderr: %s", run.Stderr)
				gotest.Empty(sub, run.Stderr)
				var answer hookAnswer
				gotest.NoError(sub, json.Unmarshal([]byte(run.Stdout), &answer), "stdout: %s", run.Stdout)
				gotest.Equal(sub, "PreToolUse", answer.HookSpecificOutput["hookEventName"])
				gotest.Contains(sub, answer.HookSpecificOutput["additionalContext"], "go test does not run them")
				gotest.Contains(sub, answer.HookSpecificOutput["additionalContext"], "\n"+row.want)
				gotest.NotContains(sub, answer.HookSpecificOutput, "permissionDecision")
			}
		})
	})

	t.When("an agent is about to run go test over packages with only stdlib tests, or a command other than go test", func(w *gotest.T) {
		w.It("exits 0 and writes nothing", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []struct {
				Desc    string
				plain   bool
				command string
			}{
				{Desc: "a package with only stdlib tests", command: "go test ./stdlib/"},
				{Desc: "a standard-library package", command: "go test strings"},
				{Desc: "a test binary built, not run", command: "go test -c ./finding/"},
				{Desc: "a module without gotest", plain: true, command: "go test ./..."},
				{Desc: "a command other than go test", command: "go vet ./... && echo test"},
			}) {
				dir := s.module
				if row.plain {
					dir = s.plain
				}
				event := hookEvent(sub, "PreToolUse", dir, map[string]string{"command": row.command})
				run, err := testkit.RunCLIWithInput(sub.Context(), dir, event, "hook", "pre-shell")
				gotest.NoError(sub, err)
				gotest.Equal(sub, 0, run.Exit, "stderr: %s", run.Stderr)
				gotest.Empty(sub, run.Stdout)
				gotest.Empty(sub, run.Stderr)
			}
		})
	})
}
