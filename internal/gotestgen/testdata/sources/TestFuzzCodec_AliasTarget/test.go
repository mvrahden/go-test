package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type Inner struct {
	N    int
	Note string
}

// AliasOf is an alias, not a defined type — it IS Inner, so both fuzz
// targets below instantiate gotest.Fuzz with the same type.
type AliasOf = Inner

type AliasFuzzTestSuite struct{}

func (s *AliasFuzzTestSuite) TestOne(t *gotest.T) {}

func (s *AliasFuzzTestSuite) FuzzViaAlias(f *gotest.F) {
	gotest.Fuzz(f, func(t *gotest.T, v AliasOf) { gotest.True(t, v.N == v.N) })
}

func (s *AliasFuzzTestSuite) FuzzViaInner(f *gotest.F) {
	gotest.Fuzz(f, func(t *gotest.T, v Inner) { gotest.True(t, v.N == v.N) })
}
