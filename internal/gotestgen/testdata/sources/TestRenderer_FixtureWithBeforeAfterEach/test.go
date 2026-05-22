package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type EachFixture struct {}

func (f *EachFixture) BeforeAll(ctx context.Context) error  { return nil }
func (f *EachFixture) AfterAll(ctx context.Context) error   { return nil }
func (f *EachFixture) BeforeEach(ctx context.Context) error { return nil }
func (f *EachFixture) AfterEach(ctx context.Context) error  { return nil }

type EachTestSuite struct {
	*EachFixture
}

func (s *EachTestSuite) BeforeAll(t *gotest.T)  {}
func (s *EachTestSuite) AfterAll(t *gotest.T)   {}
func (s *EachTestSuite) BeforeEach(t *gotest.T) {}
func (s *EachTestSuite) AfterEach(t *gotest.T)  {}
func (s *EachTestSuite) TestCase(t *gotest.T)   {}
