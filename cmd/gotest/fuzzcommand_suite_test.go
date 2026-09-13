package main_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// FuzzCommandTestSuite drives 'gotest fuzz' and 'gotest scaffold fuzz' through the built binary.
//
//nolint:lifecycle-pair // BeforeAll only wraps the shared binary, which its fixture removes
type FuzzCommandTestSuite struct {
	CLI *gotestcli.BinarySharedFixture
	cli cliRunner
}

func (s *FuzzCommandTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *FuzzCommandTestSuite) BeforeAll(t *gotest.T) {
	s.cli = newCLIRunner(s.CLI)
}

func (s *FuzzCommandTestSuite) TestFuzzSubcommand(t *gotest.T) {
	t.When("a session ends", func(w *gotest.T) {
		summaryPath := filepath.Join(w.TempDir(), "summary.md")
		env := []string{"GITHUB_ACTIONS=true", "GITHUB_STEP_SUMMARY=" + summaryPath}
		out, code := s.cli.runEnv(w, env, "fuzz", "--for=10s", "--target=FuzzNotificationServiceTestSuite_FuzzTrim", "./examples/notification")
		gotest.Equal(w, 0, code, "session output:\n%s", out)

		w.It("prints the schedule and the deadline that follows it", func(it *gotest.T) {
			gotest.Contains(it, out, "fuzzing 1 target(s), 1 at a time, 10s each (~10s wall-clock, hard stop at 2m10s)")
		})
		w.It("closes with one line saying what ran and what was found", func(it *gotest.T) {
			gotest.Regexp(it, `fuzzed 1 target in \d+\.\ds: [\d,]+ execs, \d+ new interesting inputs?, no crashers`, out)
		})
		w.It("writes the session to the GitHub step summary", func(it *gotest.T) {
			summary, err := os.ReadFile(summaryPath)
			gotest.NoError(it, err)
			gotest.Regexp(it, `### Fuzzed 1 target in \d+\.\ds — no crashers`, string(summary))
			gotest.Contains(it, string(summary), "| FuzzNotificationServiceTestSuite_FuzzTrim | ")
		})
	})
	t.It("refuses --timeout, since --for is the session's only clock", func(it *gotest.T) {
		out, code := s.cli.runExit(it, "fuzz", "--timeout=5m", "./examples/notification")
		gotest.Equal(it, 2, code, "output:\n%s", out)
		gotest.Contains(it, out, "--timeout")
		gotest.Contains(it, out, "--for")
	})
	t.It("reports when no fuzz targets exist", func(it *gotest.T) {
		out, code := s.cli.runExit(it, "fuzz", "./internal/protocol")
		gotest.Contains(it, out, "no fuzz targets found")
		gotest.Equal(it, 0, code)
	})
}

// runScaffoldFuzzCLI writes files (module + a single "codec.go" source) to
// an isolated temp module and runs "gotest scaffold --fuzz" from inside it,
// so the command's writeScaffoldFile output never touches the real repo.
func (s *FuzzCommandTestSuite) runScaffoldFuzzCLI(t *gotest.T, codecSrc, funcName string) (string, int, string) { //nolint:gocritic // hugeParam: test helper
	dir := t.TempDir()
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module fuzzscaffold\n\ngo 1.24\n"), 0644)) //nolint:gosec // G306: throwaway test module
	gotest.NoError(t, os.MkdirAll(filepath.Join(dir, "codec"), 0755))
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "codec", "codec.go"), []byte(codecSrc), 0644)) //nolint:gosec // G306: throwaway test module

	out, code := runGotestIn(t, s.cli.binary, dir, nil, "scaffold", "--fuzz", "./codec."+funcName)
	return out, code, dir
}

func (s *FuzzCommandTestSuite) TestScaffoldFuzzSubcommand(t *gotest.T) {
	t.It("generates a round-trip skeleton for a found inverse pair", func(it *gotest.T) {
		out, code, dir := s.runScaffoldFuzzCLI(it, `package codec

func Encode(s string) ([]byte, error) { return []byte(s), nil }
func Decode(b []byte) (string, error) { return string(b), nil }
`, "Encode")
		gotest.Equal(it, 0, code)
		gotest.Contains(it, out, "Generated: "+filepath.Join("codec", "encode_fuzz_test.go"))

		generated, err := os.ReadFile(filepath.Join(dir, "codec", "encode_fuzz_test.go"))
		gotest.NoError(it, err)
		src := string(generated)
		gotest.Contains(it, src, "f.Fuzz(")
		gotest.Contains(it, src, "Encode")
		gotest.Contains(it, src, "Decode")
		gotest.Contains(it, src, "gotest.Equal(t, in, decoded) // round-trip property")
	})

	t.It("falls back to a crash-safety skeleton when no inverse pair exists", func(it *gotest.T) {
		out, code, dir := s.runScaffoldFuzzCLI(it, `package codec

func Render(n int) string { return "" }
`, "Render")
		gotest.Equal(it, 0, code)
		gotest.Contains(it, out, "no inverse pair found for Render — generated crash-safety skeleton")
		gotest.Contains(it, out, "Generated: "+filepath.Join("codec", "render_fuzz_test.go"))

		generated, err := os.ReadFile(filepath.Join(dir, "codec", "render_fuzz_test.go"))
		gotest.NoError(it, err)
		src := string(generated)
		gotest.Contains(it, src, "f.Fuzz(")
		gotest.Contains(it, src, "Render(in)")
	})

	t.It("scaffolds a real skeleton for a codec-fuzzable struct parameter", func(it *gotest.T) {
		out, code, dir := s.runScaffoldFuzzCLI(it, `package codec

type Config struct{ Name string }

func ApplyConfig(c Config) string { return c.Name }
`, "ApplyConfig")
		gotest.Equal(it, 0, code)
		gotest.Contains(it, out, "no inverse pair found for ApplyConfig — generated crash-safety skeleton")
		gotest.Contains(it, out, "Generated: "+filepath.Join("codec", "apply_config_fuzz_test.go"))

		generated, err := os.ReadFile(filepath.Join(dir, "codec", "apply_config_fuzz_test.go"))
		gotest.NoError(it, err)
		src := string(generated)
		gotest.Contains(it, src, "f.Fuzz(")
		gotest.Contains(it, src, "f.Add(Config{})")
	})

	t.It("falls back to a TODO stub carrying the codec emitter's rejection", func(it *gotest.T) {
		out, code, dir := s.runScaffoldFuzzCLI(it, `package codec

func ApplyOptions(opts map[string]string) string { return opts["name"] }
`, "ApplyOptions")
		gotest.Equal(it, 0, code)
		gotest.Contains(it, out, "cannot fuzz map[string]string for ApplyOptions — generated TODO stub: ")
		gotest.Contains(it, out, "maps have no canonical encoding")
		gotest.Contains(it, out, "Generated: "+filepath.Join("codec", "apply_options_fuzz_test.go"))

		generated, err := os.ReadFile(filepath.Join(dir, "codec", "apply_options_fuzz_test.go"))
		gotest.NoError(it, err)
		src := string(generated)
		gotest.NotContains(it, src, "f.Fuzz(")
		gotest.Contains(it, src, "maps have no canonical encoding")
	})
}
