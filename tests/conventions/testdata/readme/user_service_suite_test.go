package readme

import "github.com/mvrahden/go-test/pkg/gotest"

type UserServiceTestSuite struct {
	svc *UserService
}

func (s *UserServiceTestSuite) BeforeEach(t *gotest.T) {
	s.svc = NewUserService()
}

func (s *UserServiceTestSuite) TestCreate(t *gotest.T) {
	t.It("creates a user with valid input", func(it *gotest.T) {
		err := s.svc.Create("alice@example.com")
		gotest.NoError(it, err)
	})

	t.When("email already exists", func(w *gotest.T) {
		err := s.svc.Create("bob@example.com")
		gotest.NoError(w, err)

		w.It("returns ErrDuplicate", func(it *gotest.T) {
			err := s.svc.Create("bob@example.com")
			gotest.ErrorIs(it, err, ErrDuplicate)
		})
	})
}
