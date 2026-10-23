// Package clihookshapes has suites with fuzz methods whose BeforeAll reaches
// t.T() other than by a plain call; the generator refuses each.
package clihookshapes

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type ParenTestSuite struct{}

func (s *ParenTestSuite) BeforeAll(t *gotest.T) { _ = (t).T() }

func (s *ParenTestSuite) FuzzOne(f *gotest.F) { f.Fuzz(func(t *gotest.T, in string) {}) }

type AliasTestSuite struct{}

func (s *AliasTestSuite) BeforeAll(t *gotest.T) {
	tt := t
	_ = tt.T()
}

func (s *AliasTestSuite) FuzzOne(f *gotest.F) { f.Fuzz(func(t *gotest.T, in string) {}) }

func later(func() *testing.T) {}

type MethodValueTestSuite struct{}

func (s *MethodValueTestSuite) BeforeAll(t *gotest.T) { later(t.T) }

func (s *MethodValueTestSuite) FuzzOne(f *gotest.F) { f.Fuzz(func(t *gotest.T, in string) {}) }
