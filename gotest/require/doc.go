// Package require provides test helpers that stop test execution on failure
// by calling t.FailNow(). Use these instead of manual if-checks to keep tests
// concise and readable.
//
// # Example Usage
//
//	import (
//	  "testing"
//	  "github.com/mvrahden/go-test/gotest/require"
//	)
//
//	func TestSomething(t *testing.T) {
//	  result, err := DoWork()
//	  require.NoError(t, err)
//	  require.Equal(t, "expected", result)
//	}
//
// Every function accepts optional trailing arguments for a custom failure
// message. The first argument is used as a fmt.Sprintf format string if
// multiple arguments are provided.
package require
