package gotestrunner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mvrahden/go-test/internal/protocol"
)

// gotest audit --mutate runs suites through this runner on a build that
// holds every mutant (see internal/mutate). Two variables carry what it
// needs: EnvMutantOverlay, the mutants' source the run compiles in beside
// its harness and hands its suites in GOFLAGS, so a binary a test builds
// with go build holds them too; and EnvSuiteCoverage, where a coverage run
// keeps each suite's own profile and the coverage data its children write.

// mutantOverlay reads the replacements of the overlay EnvMutantOverlay
// names; none when it is unset.
func mutantOverlay() (map[string]string, error) {
	file := os.Getenv(protocol.EnvMutantOverlay)
	if file == "" {
		return nil, nil
	}
	data, err := os.ReadFile(file) //nolint:gosec // G304: the overlay audit --mutate wrote
	if err != nil {
		return nil, fmt.Errorf("%s: %w", protocol.EnvMutantOverlay, err)
	}
	var o overlayJSON
	if err := json.Unmarshal(data, &o); err != nil {
		return nil, fmt.Errorf("%s: %s: %w", protocol.EnvMutantOverlay, file, err)
	}
	return o.Replace, nil
}

// withMutantFlags gives env's GOFLAGS the mutants' overlay, when a run
// compiles them in.
func withMutantFlags(env []string) []string {
	file := os.Getenv(protocol.EnvMutantOverlay)
	if file == "" {
		return env
	}
	flags := ""
	env = slices.DeleteFunc(env, func(kv string) bool {
		if v, ok := strings.CutPrefix(kv, "GOFLAGS="); ok {
			flags = v
			return true
		}
		return false
	})
	return append(env, "GOFLAGS="+strings.TrimSpace(flags+" -overlay="+file))
}

// SuiteCoverage is one suite process of a coverage run kept apart: its
// profile and the directory its children's coverage data went to.
type SuiteCoverage struct {
	Package  string `json:"package"`
	Suite    string `json:"suite"`
	Profile  string `json:"profile"`
	CoverDir string `json:"coverDir"`
}

// suiteCoverageIndex names the file listing a run's SuiteCoverage.
const suiteCoverageIndex = "suites.json"

// suiteCoverageDir is the directory EnvSuiteCoverage names, "" when unset.
func suiteCoverageDir() string { return os.Getenv(protocol.EnvSuiteCoverage) }

// suiteCoverDir is where a suite's children write their coverage data.
func suiteCoverDir(profile string) string { return strings.TrimSuffix(profile, ".out") + ".covdata" }

// withSuiteCoverDir points env's GOCOVERDIR at the suite's own directory
// when the run keeps each suite apart.
func withSuiteCoverDir(env []string, t *SuiteTarget) []string {
	if suiteCoverageDir() == "" || t.CoverProfile == "" {
		return env
	}
	dir := suiteCoverDir(t.CoverProfile)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return env
	}
	env = slices.DeleteFunc(slices.Clone(env), func(kv string) bool { return strings.HasPrefix(kv, "GOCOVERDIR=") })
	return append(env, "GOCOVERDIR="+dir)
}

// writeSuiteCoverage lists the suites of a run that kept them apart.
func writeSuiteCoverage(targets []SuiteTarget) {
	dir := suiteCoverageDir()
	if dir == "" {
		return
	}
	list := make([]SuiteCoverage, 0, len(targets))
	for i := range targets {
		t := &targets[i]
		list = append(list, SuiteCoverage{Package: t.Package, Suite: t.SuiteName, Profile: t.CoverProfile, CoverDir: suiteCoverDir(t.CoverProfile)})
	}
	data, err := json.Marshal(list)
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, suiteCoverageIndex), data, 0o600)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARN: suite coverage: %s\n", err)
	}
}

// ReadSuiteCoverage reads the suites a run kept apart in dir.
func ReadSuiteCoverage(dir string) ([]SuiteCoverage, error) {
	data, err := os.ReadFile(filepath.Join(dir, suiteCoverageIndex)) //nolint:gosec // G304: the index a run wrote
	if err != nil {
		return nil, err
	}
	var list []SuiteCoverage
	return list, json.Unmarshal(data, &list)
}
