package testgen_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mvrahden/go-test/about"
	"github.com/mvrahden/go-test/internal/cmd/testgen"
	"github.com/stretchr/testify/require"
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
	for idx, tC := range testcases {
		t.Run(fmt.Sprintf("Generate (idx: %d %q)", idx, tC.desc), func(t *testing.T) {
			cwd, err := os.Getwd()
			require.NoError(t, err)

			path := filepath.Join(cwd, "testdata", tC.dirName)
			results, err := testgen.GenerateSuites(path)
			require.NoError(t, err)
			require.True(t, strings.HasSuffix(results[0].AbsPath, "go-test/internal/cmd/testgen/testdata/"+tC.dirName))
			require.Equal(t, "github.com/mvrahden/go-test/internal/cmd/testgen/testdata/"+tC.dirName, results[0].Package)

			// Assert generate suite
			for idx, golden := range tC.goldenFiles {
				expected, err := os.ReadFile(filepath.Join("testdata", tC.dirName, golden))
				require.NoError(t, err)
				if idx == 0 {
					require.Equal(t, string(expected), string(results[0].PTest))
				}
				if idx == 1 {
					require.Equal(t, string(expected), string(results[0].PXTest))
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
		{"Not exist: nothing to generate", []string{"testdata/nothing-here"}},
		{"No Go-Module support: nothing to generate", []string{"strings"}},
		{"No Go-Module support: nothing to generate", []string{"net/http"}},
	}
	for _, tC := range testcases {
		t.Run(tC.desc, func(t *testing.T) {
			res, err := testgen.GenerateSuites(tC.args[0])
			require.NoError(t, err)
			require.Empty(t, res)
		})
	}
}
