package gotestrunner_test

import (
	"encoding/json"
	"os"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SharedFixtureProcessTestSuite covers the setup subprocess protocol from the runner's side.
type SharedFixtureProcessTestSuite struct{}

func (s *SharedFixtureProcessTestSuite) TestSharedFixtureProcess(t *gotest.T) {
	t.When("fixtureStateEntry parsing", func(w *gotest.T) {
		w.It("parses fixture state line", func(it *gotest.T) {
			line := `{"key":"pkg.Fixture","state":{"Host":"localhost"}}`
			var entry gotestrunner.ExportFixtureStateEntry
			err := json.Unmarshal([]byte(line), &entry)
			gotest.NoError(it, err)
			gotest.Equal(it, "pkg.Fixture", entry.Key)
			gotest.NotEmpty(it, entry.State)
		})
		w.It("parses done sentinel", func(it *gotest.T) {
			line := `{"key":"_done","teardownBudget":"2m30s"}`
			var entry gotestrunner.ExportFixtureStateEntry
			err := json.Unmarshal([]byte(line), &entry)
			gotest.NoError(it, err)
			gotest.Equal(it, "_done", entry.Key)
			gotest.Equal(it, "2m30s", entry.TeardownBudget)
		})
		w.It("parses done sentinel with error", func(it *gotest.T) {
			line := `{"key":"_done","error":"one or more shared fixtures failed"}`
			var entry gotestrunner.ExportFixtureStateEntry
			err := json.Unmarshal([]byte(line), &entry)
			gotest.NoError(it, err)
			gotest.Equal(it, "_done", entry.Key)
			gotest.Equal(it, "one or more shared fixtures failed", entry.Error)
		})
	})

	t.When("WriteStateFileForKeys", func(w *gotest.T) {
		w.It("writes subset of state to file", func(it *gotest.T) {
			tmpDir := it.TempDir()
			proc := gotestrunner.ExportNewSharedFixtureProcess(tmpDir, map[string]json.RawMessage{
				"pkg.Alpha": json.RawMessage(`{"Value":"a"}`),
				"pkg.Beta":  json.RawMessage(`{"Value":"b"}`),
				"pkg.Gamma": json.RawMessage(`{"Value":"c"}`),
			})
			path, err := proc.WriteStateFileForKeys("TestSuite", []string{"pkg.Alpha", "pkg.Gamma"})
			gotest.NoError(it, err)
			gotest.Contains(it, path, "TestSuite.json")

			data, err := os.ReadFile(path)
			gotest.NoError(it, err)
			var state map[string]json.RawMessage
			gotest.NoError(it, json.Unmarshal(data, &state))
			gotest.Len(it, state, 2)
			_, hasAlpha := state["pkg.Alpha"]
			gotest.True(it, hasAlpha)
			_, hasBeta := state["pkg.Beta"]
			gotest.False(it, hasBeta)
			_, hasGamma := state["pkg.Gamma"]
			gotest.True(it, hasGamma)
		})
	})

	t.When("State", func(w *gotest.T) {
		w.It("returns only requested keys", func(it *gotest.T) {
			proc := gotestrunner.ExportNewSharedFixtureProcess("", map[string]json.RawMessage{
				"pkg.Alpha": json.RawMessage(`{"a":1}`),
				"pkg.Beta":  json.RawMessage(`{"b":2}`),
			})
			result := proc.State([]string{"pkg.Alpha"})
			gotest.Len(it, result, 1)
			_, hasAlpha := result["pkg.Alpha"]
			gotest.True(it, hasAlpha)
		})
	})
}
