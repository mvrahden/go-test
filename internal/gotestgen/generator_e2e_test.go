package gotestgen //nolint:stdlib-test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

func TestE2E_CLI(t *testing.T) {
	testcases := []struct {
		desc    string
		dirName string
		hasPX   bool
	}{
		{"no testsuite", "no_testsuite", true},
		{"simple testsuite", "testsuite", true},
		{"suite guard", "suite_guard", false},
		{"fixture lifecycle", "fixture_lifecycle", false},
	}
	for _, tC := range testcases {
		t.Run(tC.desc, func(t *testing.T) {
			cwd, err := os.Getwd()
			gotest.NoError(t, err)

			path := filepath.Join(cwd, "testdata_e2e", tC.dirName)
			loaded, err := LoadPackages([]string{path}, nil)
			gotest.NoError(t, err)
			results, _, err := GenerateFromLoaded(loaded)
			gotest.NoError(t, err)
			gotest.True(t, strings.HasSuffix(results[0].AbsPath, "go-test/internal/gotestgen/testdata_e2e/"+tC.dirName))
			gotest.Equal(t, "github.com/mvrahden/go-test/internal/gotestgen/testdata_e2e/"+tC.dirName, results[0].PkgPath)

			gotest.MatchSnapshot(t, string(results[0].PTest), tC.dirName+"-ptest")
			if tC.hasPX {
				gotest.MatchSnapshot(t, string(results[0].PXTest), tC.dirName+"-pxtest")
			}
		})
	}
}

func TestE2E_NoTestSuites(t *testing.T) {
	testcases := []struct {
		desc string
		args []string
	}{
		{"no test files", []string{"no_testfiles"}},
		{"non-existent path returns empty", []string{"testdata_e2e/nothing-here"}},
		{"stdlib package returns empty", []string{"strings"}},
		{"stdlib nested package returns empty", []string{"net/http"}},
	}
	for _, tC := range testcases {
		t.Run(tC.desc, func(t *testing.T) {
			loaded, err := LoadPackages([]string{tC.args[0]}, nil)
			gotest.NoError(t, err)
			gotest.Empty(t, loaded)
		})
	}
}
