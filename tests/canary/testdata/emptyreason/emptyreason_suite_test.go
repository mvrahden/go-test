// Package emptyreason declares how a test asserts with an empty reason,
// which fails the test.
package emptyreason

import "github.com/mvrahden/go-test/pkg/gotest"

type EmptyReasonTestSuite struct{}

func (s *EmptyReasonTestSuite) TestEmpty(t *gotest.T) { t.AssertsBy("") }
