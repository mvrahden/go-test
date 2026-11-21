// Package mark records the order the packages' suites ran in.
package mark

import (
	"os"
	"path/filepath"
)

// Ran appends name to the markers file the run under test reads back.
func Ran(name string) {
	f, err := os.OpenFile(filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "markers"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(name + "\n")
}
