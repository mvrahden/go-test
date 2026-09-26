package gotestrunner_test

import (
	"encoding/json"
	"os"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// StateFileTestSuite covers the per-suite state file a suite process reads
// its shared fixtures from: one file per suite per package, holding every
// key the suite requires.
type StateFileTestSuite struct{}

func (s *StateFileTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type stateFileCtx struct {
	dir  string
	proc *gotestrunner.SharedFixtureProcess
}

const (
	alphaKey = "example.com/fixtures.AlphaSharedFixture"
	betaKey  = "example.com/fixtures.BetaSharedFixture"
)

func (s *StateFileTestSuite) BeforeEach(t *gotest.T) *stateFileCtx {
	dir := t.TempDir()
	return &stateFileCtx{dir: dir, proc: gotestrunner.ExportNewStateProcess(dir, map[string]json.RawMessage{
		alphaKey: json.RawMessage(`{"DataPath":"/tmp/alpha"}`),
		betaKey:  json.RawMessage(`{"Label":"beta-shared"}`),
	})}
}

func readKeys(t *gotest.T, path string) map[string]json.RawMessage {
	data, err := os.ReadFile(path)
	gotest.NoError(t, err)
	var state map[string]json.RawMessage
	gotest.NoError(t, json.Unmarshal(data, &state))
	return state
}

func (s *StateFileTestSuite) TestOneFilePerSuitePerPackage(t *gotest.T, ctx *stateFileCtx) {
	t.When("two packages declare a suite of the same name with different fixtures", func(w *gotest.T) {
		pathA, err := ctx.proc.WriteSuiteStateFile("example.com/a", "TestAlphaTestSuite", []string{alphaKey})
		gotest.NoError(w, err)
		pathB, err := ctx.proc.WriteSuiteStateFile("example.com/b", "TestAlphaTestSuite", []string{betaKey})
		gotest.NoError(w, err)

		w.It("writes two files", func(it *gotest.T) {
			gotest.NotEqual(it, pathA, pathB)
		})

		w.It("keeps each suite's own keys", func(it *gotest.T) {
			gotest.Contains(it, readKeys(it, pathA), alphaKey)
			gotest.NotContains(it, readKeys(it, pathA), betaKey)
			gotest.Contains(it, readKeys(it, pathB), betaKey)
		})
	})
}

func (s *StateFileTestSuite) TestRequiredKeyWithoutState(t *gotest.T, ctx *stateFileCtx) {
	t.It("is refused, naming the key and the suite", func(it *gotest.T) {
		_, err := ctx.proc.WriteSuiteStateFile("example.com/a", "TestGammaTestSuite", []string{alphaKey, "example.com/fixtures.GammaSharedFixture"})
		gotest.ErrorContains(it, err, "example.com/fixtures.GammaSharedFixture")
		gotest.ErrorContains(it, err, "TestGammaTestSuite")
	})
}
