package twins_test

import (
	"github.com/mvrahden/go-test/cmd/gotest/testdata/static/twins"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type UserRepositoryTestSuite struct{}

func (s *UserRepositoryTestSuite) TestCreate(t *gotest.T) {
	gotest.Equal(t, "twins", twins.Name())
}

func (s *UserRepositoryTestSuite) TestRemove(t *gotest.T) {
	gotest.NotEmpty(t, twins.Name())
}
