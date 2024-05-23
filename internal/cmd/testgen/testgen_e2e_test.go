package testgen_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mvrahden/go-test/internal/cmd/testgen"
	"github.com/stretchr/testify/require"
)

func TestE2E_CLI(t *testing.T) {
	testgen.PatchDeleteOldGeneratedFileFunc(t)

	testcases := []struct {
		desc       string
		dirName    string
		goldenFile string
		args       []string
	}{
		{"no args", "testsuite", "gotest.golden", nil},
	}
	for idx, tC := range testcases {
		t.Run(fmt.Sprintf("Generate (idx: %d %q)", idx, tC.desc), func(t *testing.T) {

			tmpDir := t.TempDir()
			testgen.PatchTargetFilenameFunc(t, tmpDir)

			args := []string{
				"-dir=" + filepath.Join("testdata", tC.dirName)}
			args = append(args, tC.args...)
			actual, err := testgen.Execute(args)
			require.NoError(t, err)

			// Assert generate suite
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
			data, err := testgen.Execute(tC.args)
			require.ErrorContains(t, err, tC.msg)
			require.Zero(t, data)
		})
	}
}
