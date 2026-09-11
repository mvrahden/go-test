package fuzzplain_test

import (
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type FuzzPlainTestSuite struct{}

func (s *FuzzPlainTestSuite) TestOne(t *gotest.T) { gotest.True(t, true) }

func (s *FuzzPlainTestSuite) FuzzTrim(f *gotest.F) {
	f.Add(" a ")
	f.Fuzz(func(t *gotest.T, in string) {
		once := strings.TrimSpace(in)
		gotest.Equal(t, once, strings.TrimSpace(once))
	})
}
