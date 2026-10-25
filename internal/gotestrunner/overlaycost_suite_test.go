package gotestrunner_test

import (
	"bytes"
	"fmt"
	"os"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// OverlayCostTestSuite measures what a run spends writing its own overlay
// and removing it at its end.
type OverlayCostTestSuite struct{}

// BenchmarkOverlayOf500Packages writes and removes the overlay of 500
// packages, one 7 KB generated file each: the cost every run pays now that
// no overlay is shared.
func (s *OverlayCostTestSuite) BenchmarkOverlayOf500Packages(b *gotest.B) {
	results := make(gotestgen.GenerateResults, 500)
	for i := range results {
		results[i] = &gotestgen.GenerateResult{
			AbsPath: fmt.Sprintf("/synthetic/pkg%03d", i),
			PTest:   bytes.Repeat([]byte("x"), 7<<10),
		}
	}
	for b.Loop() {
		dir, err := gotestrunner.WriteOverlay(results)
		gotest.NoError(b, err)
		gotest.NoError(b, os.RemoveAll(dir))
	}
}
