package gotestrunner_test

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// BinaryCacheSweepCostTestSuite measures what a run's start spends sweeping
// a large test binary cache.
type BinaryCacheSweepCostTestSuite struct{}

// BenchmarkSweepOf200KeysOf60Binaries sweeps a cache of 200 keys of 60
// binaries each, every one fresh and their checkouts present, so each sweep
// reads all of it and evicts nothing.
func (s *BinaryCacheSweepCostTestSuite) BenchmarkSweepOf200KeysOf60Binaries(b *gotest.B) {
	root, err := os.MkdirTemp("", "gotest-sweepcost-")
	gotest.NoError(b, err)
	cwd, err := os.Getwd()
	gotest.NoError(b, err)
	for k := range 200 {
		dir := filepath.Join(root, "bin", fmt.Sprintf("%016x", k))
		gotest.NoError(b, os.MkdirAll(dir, 0o755))
		gotest.NoError(b, os.WriteFile(filepath.Join(dir, ".cwd"), []byte(cwd), 0o600))
		for i := range 60 {
			bin := filepath.Join(dir, fmt.Sprintf("pkg%02d.test", i))
			gotest.NoError(b, os.WriteFile(bin, []byte("binary"), 0o755))
			gotest.NoError(b, os.WriteFile(bin+".lock", nil, 0o644))
		}
	}
	for b.Loop() {
		gotestrunner.ExportSweepBinaryCache(root, 0, time.Now())
	}
	gotest.NoError(b, os.RemoveAll(root))
}
