package repository

import (
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// TestMain keeps a job of its own, quieting the default logger for the whole
// binary, and hands the run to gotestruntime.Main, which tears the package's
// fixtures down after the tests.
func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	os.Exit(gotestruntime.Main(m))
}
