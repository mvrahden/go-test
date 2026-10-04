package twice

import (
	"os"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// TestMain runs the tests a second time after Main tore the fixtures down.
func TestMain(m *testing.M) {
	gotestruntime.Main(m)
	os.Exit(m.Run())
}
