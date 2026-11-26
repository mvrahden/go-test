//go:build unix

package skills_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// PluginHooksTestSuite: the plugin's hooks hand each event to gotest hook
// and do nothing where no gotest that knows the command can be found.
type PluginHooksTestSuite struct{}

func (s *PluginHooksTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

func (s *PluginHooksTestSuite) TestTheHooksFileCallsTheWrapper(t *gotest.T) {
	data, err := os.ReadFile(filepath.Join("hooks", "hooks.json"))
	gotest.NoError(t, err)
	var file struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	gotest.NoError(t, json.Unmarshal(data, &file))
	t.When("the hooks file is read", func(w *gotest.T) {
		w.It("wires each agent event to its gotest hook event through the wrapper", func(it *gotest.T) {
			want := map[string]string{
				"PostToolUse": `sh "${CLAUDE_PLUGIN_ROOT}/hooks/gotest-hook.sh" post-edit`,
				"PreToolUse":  `sh "${CLAUDE_PLUGIN_ROOT}/hooks/gotest-hook.sh" pre-shell`,
				"Stop":        `sh "${CLAUDE_PLUGIN_ROOT}/hooks/gotest-hook.sh" stop`,
			}
			got := map[string]string{}
			for event, groups := range file.Hooks {
				for _, g := range groups {
					for _, h := range g.Hooks {
						gotest.Equal(it, "command", h.Type)
						got[event] = h.Command
					}
				}
			}
			gotest.Equal(it, want, got)
		})
		w.It("runs after file edits and before shell commands", func(it *gotest.T) {
			gotest.Equal(it, "Edit|Write", file.Hooks["PostToolUse"][0].Matcher)
			gotest.Equal(it, "Bash", file.Hooks["PreToolUse"][0].Matcher)
		})
	})
}

// wrapperRun is what one run of the wrapper left behind.
type wrapperRun struct {
	stdout, stderr string
	err            error
}

// runWrapper runs the wrapper with only the tools it needs on PATH, plus
// the files of extra, each an executable script named by its key.
func runWrapper(t *gotest.T, event, input string, extra map[string]string) wrapperRun {
	bin := t.TempDir()
	for _, tool := range []string{"cat", "grep", "dirname"} {
		path, err := exec.LookPath(tool)
		gotest.NoError(t, err)
		gotest.NoError(t, os.Symlink(path, filepath.Join(bin, tool)))
	}
	for name, body := range extra {
		gotest.NoError(t, os.WriteFile(filepath.Join(bin, name), []byte(body), 0o700)) //nolint:gosec // G306: a stub the wrapper runs
	}
	sh, err := exec.LookPath("sh")
	gotest.NoError(t, err)
	script, err := filepath.Abs(filepath.Join("hooks", "gotest-hook.sh"))
	gotest.NoError(t, err)
	cmd := exec.CommandContext(t.Context(), sh, script, event) //nolint:gosec // G204: the plugin's own wrapper
	cmd.Dir = t.TempDir()
	cmd.Env = []string{"PATH=" + bin, "HOME=" + cmd.Dir}
	cmd.Stdin = bytes.NewBufferString(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	return wrapperRun{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

const preShellEvent = `{"tool_input":{"command":"go test ./..."}}`

func (s *PluginHooksTestSuite) TestTheWrapperFindsGotest(t *gotest.T) {
	t.When("neither go nor gotest is on PATH", func(w *gotest.T) {
		run := runWrapper(w, "pre-shell", preShellEvent, nil)
		w.It("exits 0 and writes nothing", func(it *gotest.T) {
			gotest.NoError(it, run.err)
			gotest.Empty(it, run.stdout)
			gotest.Empty(it, run.stderr)
		})
	})
	t.When("the gotest on PATH does not know the hook command", func(w *gotest.T) {
		calls := filepath.Join(w.TempDir(), "calls")
		run := runWrapper(w, "pre-shell", preShellEvent, map[string]string{
			"gotest": "#!/bin/sh\necho \"$*\" >> " + calls + "\necho 'gotest — test suite runner for Go'\n",
		})
		w.It("asks only for its help, exits 0 and writes nothing", func(it *gotest.T) {
			gotest.NoError(it, run.err)
			gotest.Empty(it, run.stdout)
			gotest.Empty(it, run.stderr)
			got, err := os.ReadFile(calls) //nolint:gosec // G304: the stub's own log
			gotest.NoError(it, err)
			gotest.Equal(it, "help hook\n", string(got))
		})
	})
	t.When("the gotest on PATH knows the hook command", func(w *gotest.T) {
		run := runWrapper(w, "pre-shell", preShellEvent, map[string]string{
			"gotest": "#!/bin/sh\nif [ \"$1\" = help ]; then echo 'gotest hook — answer'; exit 0; fi\necho \"answer to $1 $2: $(cat)\"\n",
		})
		w.It("hands it the event and its input, and passes its answer on", func(it *gotest.T) {
			gotest.NoError(it, run.err)
			gotest.Equal(it, "answer to hook pre-shell: "+preShellEvent+"\n", run.stdout)
			gotest.Empty(it, run.stderr)
		})
	})
	t.When("the event is the end of the agent's turn", func(w *gotest.T) {
		calls := filepath.Join(w.TempDir(), "calls")
		run := runWrapper(w, "stop", `{"hook_event_name":"Stop"}`, map[string]string{
			"gotest": "#!/bin/sh\necho \"$*\" >> " + calls + "\n",
		})
		w.It("starts no gotest", func(it *gotest.T) {
			gotest.NoError(it, run.err)
			gotest.Empty(it, run.stdout)
			_, err := os.Stat(calls)
			gotest.ErrorIs(it, err, os.ErrNotExist)
		})
	})
	t.When("the event is a shell command that is not go test", func(w *gotest.T) {
		calls := filepath.Join(w.TempDir(), "calls")
		run := runWrapper(w, "pre-shell", `{"tool_input":{"command":"ls -la"}}`, map[string]string{
			"gotest": "#!/bin/sh\necho \"$*\" >> " + calls + "\n",
		})
		w.It("starts no gotest", func(it *gotest.T) {
			gotest.NoError(it, run.err)
			gotest.Empty(it, run.stdout)
			_, err := os.Stat(calls)
			gotest.ErrorIs(it, err, os.ErrNotExist)
		})
	})
}
