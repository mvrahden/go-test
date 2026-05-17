package repository

import "github.com/mvrahden/go-test/pkg/gotest"

type UserRepositoryTestSuite struct {
	DB   *DatabaseFixture
	repo *userRepository
}

func (s *UserRepositoryTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.IntegrationSuiteConfig()
}

func (s *UserRepositoryTestSuite) BeforeEach(t *gotest.T) {
	s.repo = newUserRepository(s.DB)
}

func (s *UserRepositoryTestSuite) TestCreateUser(t *gotest.T) {
	t.When("a new user is created", func(t *gotest.T) {
		s.repo.Create(User{ID: "1", Email: "alice@example.com", Name: "Alice"})

		t.It("can be found by ID", func(t *gotest.T) {
			user, found := s.repo.FindByID("1")
			gotest.True(t, found)
			gotest.Equal(t, "alice@example.com", user.Email)
		})
	})
}

func (s *UserRepositoryTestSuite) TestFindNonExistentUser(t *gotest.T) {
	t.When("the user does not exist", func(t *gotest.T) {
		_, found := s.repo.FindByID("nonexistent")

		t.It("returns not found", func(t *gotest.T) {
			gotest.False(t, found)
		})
	})
}

func (s *UserRepositoryTestSuite) TestDeleteUser(t *gotest.T) {
	t.When("an existing user is deleted", func(t *gotest.T) {
		s.repo.Create(User{ID: "2", Email: "bob@example.com", Name: "Bob"})
		s.repo.Delete("2")

		t.It("can no longer be found", func(t *gotest.T) {
			_, found := s.repo.FindByID("2")
			gotest.False(t, found)
		})
	})
}
