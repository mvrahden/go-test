package run

import (
	"os"
	"testing"
)

// TestMain runs the tests itself: fixtures would never be torn down.
func TestMain(m *testing.M) { os.Exit(m.Run()) }
