package orphanwarned

import "github.com/mvrahden/go-test/pkg/gotest"

// Loader is declared in a non-test file: the run leaves it alone.
type Loader struct{}

func (l *Loader) BenchmarkLoad(b *gotest.B) {}
