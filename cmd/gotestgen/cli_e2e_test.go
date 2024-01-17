package gotestgen_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mvrahden/go-test/cmd/gotestgen"
	"github.com/stretchr/testify/require"
)

func TestE2E_CLI(t *testing.T) {
	gotestgen.PatchDeleteOldGeneratedFileFunc(t)

	testcases := []struct {
		desc                   string
		dirName                string
		goldenFile             string
		args                   []string
		expectedOutputFilename string
	}{
		{"no args", "testsuite", "gotest.golden", nil, "gotest_gensuite_test.go"},
		{"define out file", "testsuite", "gotest.golden", []string{"-out=gotest_gensuite_test.abc.go"}, "gotest_gensuite_test.abc.go"},
	}
	for idx, tC := range testcases {
		t.Run(fmt.Sprintf("Generate (idx: %d %q)", idx, tC.desc), func(t *testing.T) {

			tmpDir := t.TempDir()
			gotestgen.PatchTargetFilenameFunc(t, tmpDir)
			tmpFile := filepath.Join(tmpDir, tC.expectedOutputFilename)

			args := []string{
				"-dir=" + filepath.Join("testdata", tC.dirName)}
			args = append(args, tC.args...)
			err := gotestgen.Execute(args)
			require.NoError(t, err)
			require.FileExists(t, tmpFile)

			actual, err := os.ReadFile(tmpFile)
			require.NoError(t, err)
			expected, err := os.ReadFile(filepath.Join("testdata", tC.dirName, tC.goldenFile))
			require.NoError(t, err)
			require.Equal(t, string(expected), string(actual))
		})
	}
}

func TestE2E_Errors(t *testing.T) {
	testcases := []struct {
		desc string
		args []string
		msg  string
	}{
		{
			"on no enums in given directory (wrong path)", []string{"-dir=testdata/nothing-here"}, "no such directory",
		},
	}
	for _, tC := range testcases {
		t.Run(tC.desc, func(t *testing.T) {
			err := gotestgen.Execute(tC.args)
			require.ErrorContains(t, err, tC.msg)
		})
	}
}
