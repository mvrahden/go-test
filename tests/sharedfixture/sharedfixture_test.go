package sharedfixture_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

func TestSharedFixtureIntegration(t *testing.T) {
	standalonePattern := "github.com/mvrahden/go-test/tests/sharedfixture/standalone/..."
	fixtureboundPattern := "github.com/mvrahden/go-test/tests/sharedfixture/fixturebound/..."

	allResults, allSharedFixtures, err := gotestgen.GenerateWithSharedFixtures(
		[]string{standalonePattern, fixtureboundPattern}, nil,
	)
	gotest.NoError(t, err)

	t.Run("Discovery", func(t *testing.T) {
		gotest.Equal(t, 2, len(allSharedFixtures), "expected Alpha and Beta shared fixtures")

		found := map[string]gotestgen.SharedFixtureInfo{}
		for _, sf := range allSharedFixtures {
			found[sf.Identifier] = sf
		}

		alpha, ok := found["AlphaSharedFixture"]
		gotest.True(t, ok, "expected AlphaSharedFixture")
		gotest.True(t, alpha.HasHydrate)
		gotest.True(t, alpha.HasDehydrate)
		gotest.Contains(t, alpha.TransferFields, "DataPath")
		gotest.NotContains(t, alpha.TransferFields, "Handle")
		gotest.Contains(t, alpha.LocalFields, "Handle")

		beta, ok := found["BetaSharedFixture"]
		gotest.True(t, ok, "expected BetaSharedFixture")
		gotest.False(t, beta.HasHydrate)
		gotest.False(t, beta.HasDehydrate)
		gotest.Contains(t, beta.TransferFields, "Label")
		gotest.Contains(t, beta.TransferFields, "Count")
		gotest.Empty(t, beta.LocalFields)
	})

	tmpDir, err := gotestrunner.WriteOverlay(allResults)
	gotest.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	t.Run("SetupSubprocess", func(t *testing.T) {
		setupSrc, err := gotestgen.GenerateSharedSetup(allSharedFixtures)
		gotest.NoError(t, err)

		sharedDir := filepath.Join(tmpDir, "shared")
		gotest.NoError(t, os.MkdirAll(sharedDir, 0755))
		setupFile := filepath.Join(sharedDir, "setup.go")
		gotest.NoError(t, os.WriteFile(setupFile, setupSrc, 0644))

		setupBin := filepath.Join(sharedDir, "setup")
		buildCmd := exec.Command("go", "build", "-o", setupBin, setupFile)
		buildCmd.Stderr = os.Stderr
		gotest.NoError(t, buildCmd.Run(), "build shared fixture setup binary")

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, setupBin)
		cmd.Stderr = os.Stderr

		stdout, err := cmd.StdoutPipe()
		gotest.NoError(t, err)
		gotest.NoError(t, cmd.Start())

		var state map[string]json.RawMessage
		gotest.NoError(t, json.NewDecoder(stdout).Decode(&state))

		stateBytes, err := json.Marshal(state)
		gotest.NoError(t, err)

		stateFile := filepath.Join(sharedDir, "state.json")
		gotest.NoError(t, os.WriteFile(stateFile, stateBytes, 0644))

		t.Run("TempDirStructure", func(t *testing.T) {
			_, err := os.Stat(stateFile)
			gotest.NoError(t, err, "state.json should exist in shared/")
		})

		t.Run("StateContent", func(t *testing.T) {
			gotest.Equal(t, 2, len(state), "expected entries for Alpha and Beta")

			alphaKey := "github.com/mvrahden/go-test/tests/sharedfixture/fixtures.AlphaSharedFixture"
			betaKey := "github.com/mvrahden/go-test/tests/sharedfixture/fixtures.BetaSharedFixture"

			_, hasAlpha := state[alphaKey]
			gotest.True(t, hasAlpha, "state should contain AlphaSharedFixture")
			_, hasBeta := state[betaKey]
			gotest.True(t, hasBeta, "state should contain BetaSharedFixture")

			var alphaState struct{ DataPath string }
			gotest.NoError(t, json.Unmarshal(state[alphaKey], &alphaState))
			gotest.NotEqual(t, "", alphaState.DataPath, "Alpha.DataPath should be a real temp file path")

			var betaState struct {
				Label string
				Count int
			}
			gotest.NoError(t, json.Unmarshal(state[betaKey], &betaState))
			gotest.Equal(t, "beta-shared", betaState.Label)
			gotest.Equal(t, 42, betaState.Count)
		})

		t.Run("RunTests", func(t *testing.T) {
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
			gotest.NoError(t, err)
			gotest.Equal(t, 0, code, "all tests should pass")
		})

		t.Run("RunSpecJSON", func(t *testing.T) {
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
			gotest.NoError(t, err)
			gotest.Equal(t, 0, code, "all tests should pass via JSON runner")

			events, err := gotestspec.ParseEvents(bytes.NewReader(jsonData))
			gotest.NoError(t, err)

			tree := gotestspec.BuildTree(events)
			var buf bytes.Buffer
			gotestspec.RenderTerminal(&buf, tree, gotestspec.WithNoColor())
			output := buf.String()

			gotest.Contains(t, output, "Alpha")
			gotest.Contains(t, output, "Multi")
			gotest.Contains(t, output, "Service")
		})

		cmd.Process.Signal(os.Interrupt)
		done := make(chan struct{})
		go func() { cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			cmd.Process.Kill()
			t.Fatal("shared fixture subprocess did not exit after SIGTERM")
		}
	})

	t.Run("GeneratedCode", func(t *testing.T) {
		for _, r := range allResults {
			if len(r.PTest) == 0 {
				continue
			}
			code := string(r.PTest)

			if strings.HasSuffix(r.Package, "/standalone") {
				t.Run("standalone", func(t *testing.T) {
					gotest.Contains(t, code, `os.Getenv("GOTEST_SHARED_STATE_FILE")`)
					gotest.Contains(t, code, "os.ReadFile(ƒsharedFile)")
					gotest.Contains(t, code, "s.Alpha = sf0")
					gotest.Contains(t, code, "sf0.Hydrate(context.Background())")
					gotest.Contains(t, code, "sf0.Dehydrate(context.Background())")
					gotest.Contains(t, code, "s.Beta = sf1")
					gotest.NotContains(t, code, "sf1.Hydrate")
					gotest.NotContains(t, code, "sf1.Dehydrate")
				})
			}

			if strings.HasSuffix(r.Package, "/fixturebound") {
				t.Run("fixturebound", func(t *testing.T) {
					gotest.Contains(t, code, `os.Getenv("GOTEST_SHARED_STATE_FILE")`)
					gotest.Contains(t, code, "os.ReadFile(ƒsharedFile)")
					gotest.Contains(t, code, "ƒ_InfraFixture.Alpha = sf0")
					gotest.Contains(t, code, "sf0.Hydrate(context.Background())")
				})
			}
		}
	})
}
