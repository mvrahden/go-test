// Package inner declares a type of the same name as the suite in shadow.
package inner

type ShadowTestSuite struct{}

func (s *ShadowTestSuite) TestOne() {}

var _ = ShadowTestSuite{}
