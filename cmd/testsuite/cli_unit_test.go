package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ArgsParse(t *testing.T) {
	testCases := []struct {
		givenArgs     []string
		expectedPath  string
		expectedNargs []string
		expectedError string
	}{
		{givenArgs: []string{"abc"}, expectedPath: "abc", expectedNargs: []string(nil)},
		{givenArgs: []string{"-", "def"}, expectedPath: "", expectedNargs: []string(nil), expectedError: `given "-", must provide further nargs`},
		{givenArgs: []string{".", "-", "def"}, expectedPath: ".", expectedNargs: []string{"def"}},
		{givenArgs: []string{"abc", "-", "def"}, expectedPath: "abc", expectedNargs: []string{"def"}},
	}

	for idx, tC := range testCases {
		t.Run(fmt.Sprintf("idx %d", idx), func(t *testing.T) {
			var path string
			nargs, err := parseFlags(tC.givenArgs, &path)
			if tC.expectedError == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tC.expectedError)
			}
			require.Equal(t, tC.expectedPath, path)
			require.Equal(t, tC.expectedNargs, nargs)
		})
	}
}
