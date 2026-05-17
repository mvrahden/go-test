package repository_test

import "github.com/mvrahden/go-test/pkg/gotest"

type UserRepositoryTestSuite struct {
	users map[string]string
}

func (s *UserRepositoryTestSuite) BeforeEach(t *gotest.T) {
	s.users = map[string]string{}
}

func (s *UserRepositoryTestSuite) TestCreateUser(t *gotest.T) {
	t.When("a user is created", func(t *gotest.T) {
		s.users["alice"] = "alice@example.com"

		t.It("stores the email", func(t *gotest.T) {
			gotest.Equal(t, "alice@example.com", s.users["alice"])
		})
		t.It("has one entry", func(t *gotest.T) {
			gotest.Len(t, s.users, 1)
		})
	})
}

func (s *UserRepositoryTestSuite) TestDeleteUser(t *gotest.T) {
	t.When("the only user is removed", func(t *gotest.T) {
		s.users["bob"] = "bob@example.com"
		delete(s.users, "bob")

		t.It("leaves the store empty", func(t *gotest.T) {
			gotest.Empty(t, s.users)
		})
	})
}
