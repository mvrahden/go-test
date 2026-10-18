package genpgpg

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	apg "github.com/mvrahden/go-test/tests/canary/testdata/shapes/genpgpg/a/pg"
	bpg "github.com/mvrahden/go-test/tests/canary/testdata/shapes/genpgpg/b/pg"
)

// BothTestSuite binds a fixture from each of two packages named pg.
type BothTestSuite struct {
	A *apg.DBFixture
	B *bpg.DBFixture
}

func (s *BothTestSuite) TestBoth(t *gotest.T) {
	gotest.Equal(t, "a", s.A.Name)
	gotest.Equal(t, "b", s.B.Name)
}
