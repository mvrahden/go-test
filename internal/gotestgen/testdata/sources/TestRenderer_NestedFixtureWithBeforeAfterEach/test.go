package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type InfraFixture struct {}

func (f *InfraFixture) BeforeAll(ctx context.Context) error  { return nil }
func (f *InfraFixture) BeforeEach(ctx context.Context) error { return nil }
func (f *InfraFixture) AfterEach(ctx context.Context) error  { return nil }

type APIFixture struct {
	*InfraFixture
}

func (f *APIFixture) BeforeAll(ctx context.Context) error  { return nil }
func (f *APIFixture) BeforeEach(ctx context.Context) error { return nil }
func (f *APIFixture) AfterEach(ctx context.Context) error  { return nil }

type HandlerTestSuite struct {
	*APIFixture
}

func (s *HandlerTestSuite) BeforeEach(t *gotest.T) {}
func (s *HandlerTestSuite) AfterEach(t *gotest.T)  {}
func (s *HandlerTestSuite) TestGet(t *gotest.T)    {}
