package gotestruntime_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// SharedStateTestSuite: a suite process reads its shared fixtures' state
// from the file the runner wrote.
type SharedStateTestSuite struct{}

func (s *SharedStateTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *SharedStateTestSuite) TestRead(t *gotest.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	gotest.NoError(t, os.WriteFile(path, []byte(`{"a/b.X":{"Addr":"mem://x"}}`), 0o600))
	state, err := gotestruntime.ExportLoadSharedState(path)

	t.When("a suite process reads the state file the runner wrote for it", func(w *gotest.T) {
		w.It("finds each fixture's state under its key, byte for byte", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.Equal(it, `{"Addr":"mem://x"}`, string(state["a/b.X"]))
		})
	})
}

func (s *SharedStateTestSuite) TestNotAnObject(t *gotest.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	gotest.NoError(t, os.WriteFile(path, []byte(`[`), 0o600))
	_, err := gotestruntime.ExportLoadSharedState(path)

	t.When("the state file is not a JSON object", func(w *gotest.T) {
		w.It("says the shared state could not be read", func(it *gotest.T) {
			gotest.ErrorContains(it, err, "unmarshal shared state")
		})
	})
}
