package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ArgsParse(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	testCases := []struct {
		givenArgs     []string
		expectedCfg   ExecConfig
		expectedError string
	}{
		// stdlib drop-in replacement
		{givenArgs: []string{"-ƒƒ.internal.debug", "-abc"}, expectedCfg: ExecConfig{MaxConcurrency: 8, CWD: cwd, NArgs: []string{"-abc"}}},
		{givenArgs: []string{"abc"}, expectedCfg: ExecConfig{MaxConcurrency: 8, CWD: cwd, PackageNameList: []PkgNameExec{{Name: "abc"}}, NArgs: []string{}}},
		{
			givenArgs: []string{"-timeout", "30s", "-coverprofile=/var/folders/tv/76j4l4z95m7bqj6vrq2qbyk00000gn/T/vscode-gobSiwCA/go-code-cover", "-run", "^Test_ArgsParse$", "github.com/mvrahden/go-test/cmd/testsuite", "-count=1"},
			expectedCfg: ExecConfig{
				MaxConcurrency: 8,
				CWD:            cwd,
				PackageNameList: []PkgNameExec{
					{Name: "github.com/mvrahden/go-test/cmd/testsuite"},
				},
				NArgs: []string{"-timeout", "30s", "-coverprofile=/var/folders/tv/76j4l4z95m7bqj6vrq2qbyk00000gn/T/vscode-gobSiwCA/go-code-cover", "-run", "^Test_ArgsParse$", "-count=1"}},
		},
		{ // with recursive walk from CWD
			givenArgs: []string{"-timeout", "30s", "-coverprofile=/var/folders/tv/76j4l4z95m7bqj6vrq2qbyk00000gn/T/vscode-gobSiwCA/go-code-cover", "-run", "^Test_ArgsParse$", "./...", "-count=1"},
			expectedCfg: ExecConfig{
				MaxConcurrency: 8,
				CWD:            cwd,
				PackageNameList: []PkgNameExec{
					{Name: "./...", IsRecursiveWalk: true}, //TODO: PkgName incorrect, PkgNameList not final (due to runner orchestration, need to determine each package individually)
				},
				NArgs: []string{"-timeout", "30s", "-coverprofile=/var/folders/tv/76j4l4z95m7bqj6vrq2qbyk00000gn/T/vscode-gobSiwCA/go-code-cover", "-run", "^Test_ArgsParse$", "-count=1"}},
		},
		{ // with recursive walk from pkgName
			givenArgs: []string{"-timeout", "30s", "-coverprofile=/var/folders/tv/76j4l4z95m7bqj6vrq2qbyk00000gn/T/vscode-gobSiwCA/go-code-cover", "-run", "^Test_ArgsParse$", "github.com/mvrahden/go-test/cmd/testsuite/...", "-count=1"},
			expectedCfg: ExecConfig{
				MaxConcurrency: 8,
				CWD:            cwd,
				PackageNameList: []PkgNameExec{
					{Name: "github.com/mvrahden/go-test/cmd/testsuite/...", IsRecursiveWalk: true}, //TODO: PkgName incorrect, PkgNameList not final
				},
				NArgs: []string{"-timeout", "30s", "-coverprofile=/var/folders/tv/76j4l4z95m7bqj6vrq2qbyk00000gn/T/vscode-gobSiwCA/go-code-cover", "-run", "^Test_ArgsParse$", "-count=1"}},
		},
		// {givenArgs: []string{"abc", "-", "def"}, expectedCfg: ExecConfig{MaxConcurrency: 8, CWD: cwd, NArgs: []string{"def"}}},
		// {givenArgs: []string{"./..."}, expectedCfg: ExecConfig{IsRecursiveWalk: true, MaxConcurrency: 8, RawPath: "./...", CWD: cwd}},
	}

	for idx, tC := range testCases {
		t.Run(fmt.Sprintf("idx %d", idx), func(t *testing.T) {
			cfg, err := ParseArgs(tC.givenArgs, true)
			if tC.expectedError == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tC.expectedError)
			}
			require.Equal(t, tC.expectedCfg.MaxConcurrency, cfg.MaxConcurrency)
			require.Equal(t, tC.expectedCfg.CWD, cfg.CWD)
			require.Equal(t, tC.expectedCfg.NArgs, cfg.NArgs)
			require.Equal(t, tC.expectedCfg.PackageNameList, cfg.PackageNameList)
			require.NotZero(t, cfg.SuitesGenerate)
			require.NotZero(t, cfg.SuitesRun)
			require.NotZero(t, cfg.SuitesCleanup)
		})
	}
}
