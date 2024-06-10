package testgen_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mvrahden/go-test/internal/cmd/testgen"
	"github.com/stretchr/testify/require"
)

func TestE2E_CLI(t *testing.T) {
	testgen.PatchDeleteOldGeneratedFileFunc(t)

	testcases := []struct {
		desc        string
		dirName     string
		goldenFiles []string
		args        []string
	}{
		{"no args", "testsuite", []string{"ptest_gotest.golden", "pxtest_gotest.golden"}, nil},
	}
	for idx, tC := range testcases {
		t.Run(fmt.Sprintf("Generate (idx: %d %q)", idx, tC.desc), func(t *testing.T) {

			tmpDir := t.TempDir()
			testgen.PatchTargetFilenameFunc(t, tmpDir)

			args := []string{
				"-dir=" + filepath.Join("testdata", tC.dirName)}
			args = append(args, tC.args...)
			retArgs, ptestActual, pxtestActual, err := testgen.Execute(args)
			require.NoError(t, err)
			require.True(t, strings.HasSuffix(retArgs.AbsPath, "/go-test/internal/cmd/testgen/testdata/testsuite"))
			require.Equal(t, "github.com/mvrahden/go-test/internal/cmd/testgen/testdata/testsuite", retArgs.Package)
			require.Zero(t, retArgs.Args)
			require.False(t, retArgs.SkipAutoDelete)

			// Assert generate suite
			for idx, golden := range tC.goldenFiles {
				expected, err := os.ReadFile(filepath.Join("testdata", tC.dirName, golden))
				require.NoError(t, err)
				if idx == 0 {
					require.Equal(t, string(expected), string(ptestActual))
				}
				if idx == 1 {
					require.Equal(t, string(expected), string(pxtestActual))
				}
			}
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
			_, ptest, pxtest, err := testgen.Execute(tC.args)
			require.ErrorContains(t, err, tC.msg)
			require.Zero(t, ptest)
			require.Zero(t, pxtest)
		})
	}
}
