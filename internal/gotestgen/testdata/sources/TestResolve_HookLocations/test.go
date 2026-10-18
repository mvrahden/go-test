package TestResolve_HookLocations

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
	"testpkg/extfixtures"
)

// LocalFixture declares its BeforeEach.
type LocalFixture struct{}

func (f *LocalFixture) BeforeAll(ctx context.Context) error  { return nil }
func (f *LocalFixture) BeforeEach(ctx context.Context) error { return nil }

type HookLocationsTestSuite struct {
	Local  *LocalFixture
	Remote *extfixtures.RemoteFixture
}

func (s *HookLocationsTestSuite) TestA(t *gotest.T) {}
