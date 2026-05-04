package gotestgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mvrahden/go-test/about"
	"github.com/mvrahden/go-test/pkg/gotest"
)

func TestE2E_CLI(t *testing.T) {
	testcases := []struct {
		desc        string
		dirName     string
		goldenFiles []string
	}{
		{"no testsuite", "no_testsuite", []string{about.PSuite + ".golden", about.PXSuite + ".golden"}},
		{"simple testsuite", "testsuite", []string{about.PSuite + ".golden", about.PXSuite + ".golden"}},
	}
	for _, tC := range testcases {
		t.Run(tC.desc, func(t *testing.T) {
			cwd, err := os.Getwd()
			gotest.NoError(t, err)

			path := filepath.Join(cwd, "testdata_e2e", tC.dirName)
			results, err := Generate([]string{path}, nil)
			gotest.NoError(t, err)
			gotest.True(t, strings.HasSuffix(results[0].AbsPath, "go-test/internal/gotestgen/testdata_e2e/"+tC.dirName))
			gotest.Equal(t, "github.com/mvrahden/go-test/internal/gotestgen/testdata_e2e/"+tC.dirName, results[0].Package)

			for idx, golden := range tC.goldenFiles {
				expected, err := os.ReadFile(filepath.Join("testdata_e2e", tC.dirName, golden))
				gotest.NoError(t, err)
				if idx == 0 {
					gotest.Equal(t, string(expected), string(results[0].PTest))
				}
				if idx == 1 {
					gotest.Equal(t, string(expected), string(results[0].PXTest))
				}
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
			res, err := Generate([]string{tC.args[0]}, nil)
			gotest.NoError(t, err)
			gotest.Empty(t, res)
		})
	}
}
