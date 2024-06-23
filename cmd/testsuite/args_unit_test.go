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
		expectedNargs []string
		expectedError string
	}{
		{givenArgs: []string{}, expectedCfg: ExecConfig{}, expectedNargs: []string(nil), expectedError: `missing args`},
		{givenArgs: []string{"-", "def"}, expectedCfg: ExecConfig{}, expectedNargs: []string(nil), expectedError: `given "-", must provide further nargs`},
		{givenArgs: []string{"abc"}, expectedCfg: ExecConfig{MaxConcurrency: 8, RawPath: "abc", CWD: cwd}, expectedNargs: []string(nil)},
		{givenArgs: []string{".", "-", "def"}, expectedCfg: ExecConfig{MaxConcurrency: 8, RawPath: ".", CWD: cwd}, expectedNargs: []string{"def"}},
		{givenArgs: []string{"abc", "-", "def"}, expectedCfg: ExecConfig{MaxConcurrency: 8, RawPath: "abc", CWD: cwd}, expectedNargs: []string{"def"}},
		{givenArgs: []string{"./..."}, expectedCfg: ExecConfig{TestRecursive: true, MaxConcurrency: 8, RawPath: "./...", CWD: cwd}, expectedNargs: []string(nil)},
	}

	for idx, tC := range testCases {
		t.Run(fmt.Sprintf("idx %d", idx), func(t *testing.T) {
			cfg, nargs, err := ParseArgs(tC.givenArgs)
			if tC.expectedError == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tC.expectedError)
			}
			require.Equal(t, tC.expectedCfg, cfg)
			require.Equal(t, tC.expectedNargs, nargs)
		})
	}
}
