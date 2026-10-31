package uses

import "github.com/mvrahden/go-test/pkg/gotest"

var _ UsesTestSuite

var (
	_ = UsesTestSuite{}
	_ = &UsesTestSuite{name: "x"}
	_ = new(UsesTestSuite)
	_ = []UsesTestSuite{{}, {name: "y"}}
	_ = map[string]*UsesTestSuite{"z": nil}
	_ = (*UsesTestSuite)(nil)
)

var _ interface{ TestName(*gotest.T) } = (*UsesTestSuite)(nil)

func build(from UsesTestSuite) *UsesTestSuite {
	var s UsesTestSuite = from
	return &s
}

var _ = build
