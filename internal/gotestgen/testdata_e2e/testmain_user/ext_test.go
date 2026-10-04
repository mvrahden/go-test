package testmainuser_test

import (
	"os"
	"testing"

	rt "github.com/mvrahden/go-test/pkg/gotestruntime"
)

// The developer's TestMain lives in the external package and reaches the
// runtime under an alias.
func TestMain(m *testing.M) {
	os.Setenv("LEDGER", "on")
	os.Exit(rt.Main(m))
}
