package repository

import "github.com/mvrahden/go-test/pkg/gotest"

type UserRepositoryTestSuite struct {
	DB *DatabaseFixture
}

func (s *UserRepositoryTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// repoCtx is per test; the fixture store is shared, so each test uses its own user IDs.
type repoCtx struct {
	repo *userRepository
}

func (s *UserRepositoryTestSuite) BeforeEach(t *gotest.T) *repoCtx {
	return &repoCtx{repo: newUserRepository(s.DB)}
}

func (s *UserRepositoryTestSuite) TestCreateUser(t *gotest.T, ctx *repoCtx) {
	t.When("a new user is created", func(t *gotest.T) {
		ctx.repo.Create(User{ID: "1", Email: "alice@example.com", Name: "Alice"})

		t.It("can be found by ID", func(t *gotest.T) {
			user, found := ctx.repo.FindByID("1")
			gotest.True(t, found)
			gotest.Equal(t, "alice@example.com", user.Email)
		})
	})
}

func (s *UserRepositoryTestSuite) TestFindNonExistentUser(t *gotest.T, ctx *repoCtx) {
	t.When("the user does not exist", func(t *gotest.T) {
		_, found := ctx.repo.FindByID("nonexistent")

		t.It("returns not found", func(t *gotest.T) {
			gotest.False(t, found)
		})
	})
}

func (s *UserRepositoryTestSuite) TestDeleteUser(t *gotest.T, ctx *repoCtx) {
	t.When("an existing user is deleted", func(t *gotest.T) {
		ctx.repo.Create(User{ID: "2", Email: "bob@example.com", Name: "Bob"})
		ctx.repo.Delete("2")

		t.It("can no longer be found", func(t *gotest.T) {
			_, found := ctx.repo.FindByID("2")
			gotest.False(t, found)
		})
	})
}
