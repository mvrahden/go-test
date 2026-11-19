// Package skiplog logs a line before each of its skips; see the spec
// conformance suite.
package skiplog

import "testing"

type SkipLogTestSuite struct{}

func (s *SkipLogTestSuite) TestLogsThenSkips(t *testing.T) {
	t.Log("connecting to the database")
	t.Skip("no database here")
}

func (s *SkipLogTestSuite) TestLogsThenSkipsNow(t *testing.T) {
	t.Log("checked the flag")
	t.SkipNow()
}
