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
		{"no test files", "no_testfiles", []string{about.PSuite + ".golden", about.PXSuite + ".golden"}, nil},
		{"no testsuite", "no_testsuite", []string{about.PSuite + ".golden", about.PXSuite + ".golden"}, nil},
		{"simple testsuite", "testsuite", []string{about.PSuite + ".golden", about.PXSuite + ".golden"}, nil},
	}
	for idx, tC := range testcases {
		t.Run(fmt.Sprintf("Generate (idx: %d %q)", idx, tC.desc), func(t *testing.T) {
			cwd, err := os.Getwd()
			require.NoError(t, err)

			path := filepath.Join(cwd, "testdata", tC.dirName)
			result, err := testgen.GenerateSuites(path)
			require.NoError(t, err)
			require.True(t, strings.HasSuffix(result.AbsPath, "go-test/internal/cmd/testgen/testdata/"+tC.dirName))
			require.Equal(t, "github.com/mvrahden/go-test/internal/cmd/testgen/testdata/"+tC.dirName, result.Package)

			// Assert generate suite
			for idx, golden := range tC.goldenFiles {
				expected, err := os.ReadFile(filepath.Join("testdata", tC.dirName, golden))
				require.NoError(t, err)
				if idx == 0 {
					require.Equal(t, string(expected), string(result.PTest))
				}
				if idx == 1 {
					require.Equal(t, string(expected), string(result.PXTest))
				}
			}
		})
	}
}

func TestE2E_Errors(t *testing.T) {
	testcases := []struct {
		desc     string
		args     []string
		errorMsg string
	}{
		{
			"not a Go package (no sources)", []string{"testdata/nothing-here"}, "not a Go package",
		},
	}
	for _, tC := range testcases {
		t.Run(tC.desc, func(t *testing.T) {
			res, err := testgen.GenerateSuites(tC.args[0])
			require.ErrorContains(t, err, tC.errorMsg)
			require.Zero(t, res)
		})
	}
}

func TestE2E_NoTestSuites(t *testing.T) {
	testcases := []struct {
		desc   string
		args   []string
		result testgen.GenerateResult
	}{
		{
			"nothing to generate", []string{"strings"}, testgen.GenerateResult{AbsPath: "strings", Package: "strings"},
		},
		{
			"nothing to generate", []string{"net/http"}, testgen.GenerateResult{AbsPath: "net/http", Package: "net/http"},
		},
	}
	for _, tC := range testcases {
		t.Run(tC.desc, func(t *testing.T) {
			res, err := testgen.GenerateSuites(tC.args[0])
			require.NoError(t, err)
			require.Equal(t, string(tC.result.PTest), string(res.PTest))
			require.Equal(t, tC.result, res)
		})
	}
}
