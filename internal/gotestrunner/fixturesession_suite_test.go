package gotestrunner_test

import (
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FixtureSessionTestSuite covers what a suite gets from a run's shared
// fixture session: its environment and the fixture it is given up for.
type FixtureSessionTestSuite struct{}

func (s *FixtureSessionTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *FixtureSessionTestSuite) TestEnv(t *gotest.T) {
	dir := t.TempDir()
	proc := gotestrunner.ExportNewStateProcess(dir, map[string]json.RawMessage{"p.A": json.RawMessage(`{"x":1}`)})
	f := gotestrunner.ExportNewFixtureSession(proc, nil, false)
	base := []string{"K=V"}

	t.When("the suite reads no fixture", func(w *gotest.T) {
		w.It("hands back the base environment itself", func(it *gotest.T) {
			env, err := gotestrunner.ExportSessionEnv(f, base, "example.com/a", "TestXTestSuite", nil)
			gotest.NoError(it, err)
			gotest.Equal(it, base, env)
		})
	})
	t.When("the suite reads a fixture that is up", func(w *gotest.T) {
		w.It("appends the state file of exactly its keys and leaves the base alone", func(it *gotest.T) {
			// Room to spare: an append in place would write this suite's
			// entry where the next suite's append lands.
			roomy := append(make([]string, 0, 2), "K=V")
			env, err := gotestrunner.ExportSessionEnv(f, roomy, "example.com/a", "TestXTestSuite", []string{"p.A"})
			gotest.NoError(it, err)
			gotest.Len(it, env, 2)
			gotest.Regexp(it, "^"+protocol.EnvSharedStateFile+"=", env[1])
			data, readErr := os.ReadFile(strings.TrimPrefix(env[1], protocol.EnvSharedStateFile+"="))
			gotest.NoError(it, readErr)
			gotest.JSONEq(it, `{"p.A":{"x":1}}`, data)
			gotest.Equal(it, []string{"K=V", ""}, roomy[:2])
		})
	})
	t.When("the suite reads a key with no state", func(w *gotest.T) {
		w.It("returns the state file's refusal", func(it *gotest.T) {
			_, err := gotestrunner.ExportSessionEnv(f, base, "example.com/a", "TestXTestSuite", []string{"p.Missing"})
			gotest.Error(it, err)
		})
	})
}

func (s *FixtureSessionTestSuite) TestMissing(t *gotest.T) {
	t.It("names the first key when no process ever started", func(it *gotest.T) {
		f := gotestrunner.ExportNewFixtureSession(nil, errors.New("did not start"), false)
		gotest.Equal(it, "p.A", gotestrunner.ExportSessionMissing(f, []string{"p.A", "p.B"}))
	})
	t.It("names nothing for a suite that reads nothing", func(it *gotest.T) {
		f := gotestrunner.ExportNewFixtureSession(nil, nil, false)
		gotest.Equal(it, "", gotestrunner.ExportSessionMissing(f, nil))
	})
}
