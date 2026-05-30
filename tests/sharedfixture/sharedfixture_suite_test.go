package sharedfixture_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SharedFixtureIntegrationTestSuite tests shared fixture lifecycle integration
// with real package loading, code generation, and binary execution.
type SharedFixtureIntegrationTestSuite struct{}

func (s *SharedFixtureIntegrationTestSuite) TestSharedFixtureIntegration(t *gotest.T) {
	standalonePattern := "github.com/mvrahden/go-test/tests/sharedfixture/standalone/..."
	fixtureboundPattern := "github.com/mvrahden/go-test/tests/sharedfixture/fixturebound/..."

	loaded, err := gotestgen.LoadPackages([]string{standalonePattern, fixtureboundPattern}, nil)
	gotest.NoError(t, err)
	allResults, allSharedFixtures, err := gotestgen.GenerateFromLoaded(loaded)
	gotest.NoError(t, err)

	t.When("Discovery", func(w *gotest.T) {
		gotest.Equal(w, 2, len(allSharedFixtures), "expected Alpha and Beta shared fixtures")

		found := map[string]gotestgen.SharedFixtureInfo{}
		for _, sf := range allSharedFixtures {
			found[sf.Identifier] = sf
		}

		alpha, ok := found["AlphaSharedFixture"]
		gotest.True(w, ok, "expected AlphaSharedFixture")
		gotest.True(w, alpha.HasHydrate)
		gotest.True(w, alpha.HasDehydrate)
		gotest.Contains(w, alpha.TransferFields, "DataPath")
		gotest.NotContains(w, alpha.TransferFields, "Handle")
		gotest.Contains(w, alpha.LocalFields, "Handle")

		beta, ok := found["BetaSharedFixture"]
		gotest.True(w, ok, "expected BetaSharedFixture")
		gotest.False(w, beta.HasHydrate)
		gotest.False(w, beta.HasDehydrate)
		gotest.Contains(w, beta.TransferFields, "Label")
		gotest.Contains(w, beta.TransferFields, "Count")
		gotest.Empty(w, beta.LocalFields)
	})

	tmpDir, err := gotestrunner.WriteOverlay(allResults)
	gotest.NoError(t, err)
	t.T().Cleanup(func() { os.RemoveAll(tmpDir) })

	t.When("SetupSubprocess", func(w *gotest.T) {
		setupSrc, err := gotestgen.GenerateSharedSetup(allSharedFixtures)
		gotest.NoError(w, err)

		sharedDir := filepath.Join(tmpDir, "shared")
		gotest.NoError(w, os.MkdirAll(sharedDir, 0755))
		setupFile := filepath.Join(sharedDir, "setup.go")
		gotest.NoError(w, os.WriteFile(setupFile, setupSrc, 0644))

		setupBin := filepath.Join(sharedDir, "setup")
		if runtime.GOOS == "windows" {
			setupBin += ".exe"
		}
		buildCmd := exec.Command("go", "build", "-o", setupBin, setupFile)
		buildCmd.Stderr = os.Stderr
		gotest.NoError(w, buildCmd.Run(), "build shared fixture setup binary")

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, setupBin)
		cmd.Stderr = os.Stderr
		gotestrunner.SetProcessGroup(cmd)

		stdout, err := cmd.StdoutPipe()
		gotest.NoError(w, err)
		gotest.NoError(w, cmd.Start())

		var state map[string]json.RawMessage
		gotest.NoError(w, json.NewDecoder(stdout).Decode(&state))

		stateBytes, err := json.Marshal(state)
		gotest.NoError(w, err)

		stateFile := filepath.Join(sharedDir, "state.json")
		gotest.NoError(w, os.WriteFile(stateFile, stateBytes, 0644))

		w.It("TempDirStructure", func(it *gotest.T) {
			_, err := os.Stat(stateFile)
			gotest.NoError(it, err, "state.json should exist in shared/")
		})

		w.It("StateContent", func(it *gotest.T) {
			gotest.Equal(it, 3, len(state), "expected entries for Alpha, Beta, and _teardownBudget")

			alphaKey := "github.com/mvrahden/go-test/tests/sharedfixture/fixtures.AlphaSharedFixture"
			betaKey := "github.com/mvrahden/go-test/tests/sharedfixture/fixtures.BetaSharedFixture"

			_, hasAlpha := state[alphaKey]
			gotest.True(it, hasAlpha, "state should contain AlphaSharedFixture")
			_, hasBeta := state[betaKey]
			gotest.True(it, hasBeta, "state should contain BetaSharedFixture")
			_, hasBudget := state["_teardownBudget"]
			gotest.True(it, hasBudget, "state should contain _teardownBudget")

			var alphaState struct{ DataPath string }
			gotest.NoError(it, json.Unmarshal(state[alphaKey], &alphaState))
			gotest.NotEqual(it, "", alphaState.DataPath, "Alpha.DataPath should be a real temp file path")

			var betaState struct {
				Label string
				Count int
			}
			gotest.NoError(it, json.Unmarshal(state[betaKey], &betaState))
			gotest.Equal(it, "beta-shared", betaState.Label)
			gotest.Equal(it, 42, betaState.Count)
		})

		w.It("RunTests", func(it *gotest.T) {
			overlayFlag := "-overlay=" + filepath.Join(tmpDir, "overlay.json")
			goTestArgs := []string{
				overlayFlag, "-v",
				standalonePattern,
				fixtureboundPattern,
			}
			extraEnv := map[string]string{
				"GOTEST_SHARED_STATE_FILE": stateFile,
			}

			code, err := gotestrunner.StdlibRunTests(context.Background(), goTestArgs, extraEnv)
			gotest.NoError(it, err)
			gotest.Equal(it, 0, code, "all tests should pass")
		})

		w.It("RunSpecJSON", func(it *gotest.T) {
			overlayFlag := "-overlay=" + filepath.Join(tmpDir, "overlay.json")
			goTestArgs := []string{
				overlayFlag,
				standalonePattern,
				fixtureboundPattern,
			}
			extraEnv := map[string]string{
				"GOTEST_SHARED_STATE_FILE": stateFile,
			}

			jsonData, code, err := gotestrunner.StdlibRunTestsJSON(context.Background(), goTestArgs, extraEnv)
			gotest.NoError(it, err)
			gotest.Equal(it, 0, code, "all tests should pass via JSON runner")

			events, err := gotestspec.ParseEvents(bytes.NewReader(jsonData))
			gotest.NoError(it, err)

			tree := gotestspec.BuildTree(events)
			var buf bytes.Buffer
			gotestspec.RenderTerminal(&buf, tree, gotestspec.WithNoColor())
			output := buf.String()

			gotest.Contains(it, output, "Alpha")
			gotest.Contains(it, output, "Multi")
			gotest.Contains(it, output, "Service")
		})

		gotestrunner.TerminateProcessGroup(cmd.Process.Pid)
		doneCh := make(chan struct{})
		go func() { cmd.Wait(); close(doneCh) }()
		select {
		case <-doneCh:
		case <-time.After(10 * time.Second):
			gotestrunner.ForceKillProcessGroup(cmd.Process.Pid)
			w.T().Fatal("shared fixture subprocess did not exit after termination signal")
		}
	})

	t.When("GeneratedCode", func(w *gotest.T) {
		for _, r := range allResults {
			if len(r.PTest) == 0 {
				continue
			}
			code := string(r.PTest)

			if strings.HasSuffix(r.PkgPath, "/standalone") {
				w.It("standalone", func(it *gotest.T) {
					gotest.Contains(it, code, "gotestruntime.RunFixtureMain(m,")
					gotest.Contains(it, code, "var ƒ_sf_fixtures_AlphaSharedFixture = &fixtures.AlphaSharedFixture{}")
					gotest.Contains(it, code, "var ƒ_sf_fixtures_BetaSharedFixture = &fixtures.BetaSharedFixture{}")
					gotest.Contains(it, code, "s.Alpha = ƒ_sf_fixtures_AlphaSharedFixture")
					gotest.Contains(it, code, "s.Beta = ƒ_sf_fixtures_BetaSharedFixture")
					gotest.NotContains(it, code, "encoding/json", "should NOT import encoding/json")
					gotest.NotContains(it, code, "json.Unmarshal", "should NOT inline JSON unmarshal")
				})
			}

			if strings.HasSuffix(r.PkgPath, "/fixturebound") {
				w.It("fixturebound", func(it *gotest.T) {
					gotest.Contains(it, code, "gotestruntime.RunFixtureMain(m,")
					gotest.Contains(it, code, "var ƒ_sf_fixtures_AlphaSharedFixture = &fixtures.AlphaSharedFixture{}")
					gotest.Contains(it, code, "Alpha: ƒ_sf_fixtures_AlphaSharedFixture")
					gotest.Contains(it, code, "ƒ_sf_fixtures_AlphaSharedFixture.Hydrate(ctx)")
					gotest.Contains(it, code, "ƒ_sf_fixtures_AlphaSharedFixture.Dehydrate(ctx)")
				})
			}
		}
	})
}
