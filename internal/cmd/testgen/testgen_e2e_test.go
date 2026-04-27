package testgen_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mvrahden/go-test/about"
	"github.com/mvrahden/go-test/internal/cmd/testgen"
	"github.com/mvrahden/go-test/pkg/gotest"
)

func TestE2E_CLI(t *testing.T) {
	testcases := []struct {
		desc        string
		dirName     string
		goldenFiles []string
		args        []string
	}{
		{"no testsuite", "no_testsuite", []string{about.PSuite + ".golden", about.PXSuite + ".golden"}, nil},
		{"simple testsuite", "testsuite", []string{about.PSuite + ".golden", about.PXSuite + ".golden"}, nil},
	}
	for _, tC := range testcases {
		t.Run(tC.desc, func(t *testing.T) {
			cwd, err := os.Getwd()
			gotest.NoError(t, err)

			path := filepath.Join(cwd, "testdata", tC.dirName)
			results, err := testgen.GenerateSuites(path)
			gotest.NoError(t, err)
			gotest.True(t, strings.HasSuffix(results[0].AbsPath, "go-test/internal/cmd/testgen/testdata/"+tC.dirName))
			gotest.Equal(t, "github.com/mvrahden/go-test/internal/cmd/testgen/testdata/"+tC.dirName, results[0].Package)

			// Assert generate suite
			for idx, golden := range tC.goldenFiles {
				expected, err := os.ReadFile(filepath.Join("testdata", tC.dirName, golden))
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
		{"non-existent path returns empty", []string{"testdata/nothing-here"}},
		{"stdlib package returns empty", []string{"strings"}},
		{"stdlib nested package returns empty", []string{"net/http"}},
	}
	for _, tC := range testcases {
		t.Run(tC.desc, func(t *testing.T) {
			res, err := testgen.GenerateSuites(tC.args[0])
			gotest.NoError(t, err)
			gotest.Empty(t, res)
		})
	}
}
