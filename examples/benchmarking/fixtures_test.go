package benchmarking

import (
	"context"
	"fmt"
)

// corpusSize is the number of key/value pairs KeyCorpusFixture generates. It
// serves two purposes: it's the capacity CacheTestSuite's BeforeEach gives
// the warmed cache (the corpus exactly fills it), and it's the fill size
// BenchmarkFillFromEmpty uses to build a cache from scratch one op at a
// time (see suite_test.go's fillSize).
const corpusSize = 4096

// KeyCorpusFixture builds a deterministic key/value corpus once for the
// whole package, so no benchmark pays for generating its own inputs, and
// every run reads from the same data — which is what makes --against
// comparisons meaningful.
//
// Fixtures bound to a benchmark suite may define BeforeAll/AfterAll only:
// gotest rejects per-method fixture hooks (BeforeEach/AfterEach on the
// fixture itself) for benchmark suites at generation time, because they
// would run inside the timed method's lifecycle instead of around it —
// exactly the leak the suite's own BeforeEach fencing (see suite_test.go)
// is designed to avoid.
type KeyCorpusFixture struct {
	Keys   []string
	Values []string
}

func (f *KeyCorpusFixture) BeforeAll(ctx context.Context) error {
	f.Keys = make([]string, corpusSize)
	f.Values = make([]string, corpusSize)
	for i := range corpusSize {
		f.Keys[i] = fmt.Sprintf("key-%04d", i)
		f.Values[i] = fmt.Sprintf("value-%04d", i)
	}
	return nil
}
