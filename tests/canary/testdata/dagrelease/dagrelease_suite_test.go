package dagrelease

import "github.com/mvrahden/go-test/pkg/gotest"

// BulkTestSuite reads the fixture only the bulk needs.
type BulkTestSuite struct {
	Bulk *BulkSharedFixture
}

func (s *BulkTestSuite) TestOne(t *gotest.T) { mark("bulk ran") }

// TailTestSuite runs alone, after the bulk, and reads Child and B.
type TailTestSuite struct {
	Child *ChildSharedFixture
	B     *BSharedFixture
}

func (s *TailTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Exclusive = true
	return cfg
}

func (s *TailTestSuite) TestReadsThem(t *gotest.T) {
	gotest.NotZero(t, s.Child.A)
	mark("tail ran")
}
