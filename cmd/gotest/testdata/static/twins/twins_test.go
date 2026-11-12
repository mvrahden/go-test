package twins

import "github.com/mvrahden/go-test/pkg/gotest"

type UserRepositoryTestSuite struct{}

func (s *UserRepositoryTestSuite) TestCreate(t *gotest.T) {
	gotest.Equal(t, "twins", Name())
}
