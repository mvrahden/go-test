package wrapped

import (
	"os"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// verify stands in for goleak.VerifyTestMain: it runs the tests itself through
// the interface it takes, then checks something about the whole process.
func verify(m interface{ Run() int }) {
	code := m.Run()
	logLine("verified")
	os.Exit(code)
}

func TestMain(m *testing.M) { verify(gotestruntime.M(m)) }
