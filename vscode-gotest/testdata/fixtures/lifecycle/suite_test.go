package lifecycle

import "github.com/mvrahden/go-test/pkg/gotest"

// A suite whose every lifecycle hook runs cleanly. Resources are acquired in
// BeforeEach/BeforeAll and released in the matching teardown hooks.
type LifecycleTestSuite struct {
	conn  string
	calls []string
}

func (s *LifecycleTestSuite) BeforeAll(t *gotest.T) {
	s.conn = "open"
}

func (s *LifecycleTestSuite) AfterAll(t *gotest.T) {
	s.conn = "closed"
}

func (s *LifecycleTestSuite) BeforeEach(t *gotest.T) {
	s.calls = append(s.calls, "before")
}

func (s *LifecycleTestSuite) AfterEach(t *gotest.T) {
	s.calls = nil
}

func (s *LifecycleTestSuite) TestConnection(t *gotest.T) {
	t.When("the shared connection is set up", func(t *gotest.T) {
		t.It("is open for the test", func(t *gotest.T) {
			gotest.Equal(t, "open", s.conn)
		})
		t.It("ran the per-test hook", func(t *gotest.T) {
			gotest.Equal(t, 1, len(s.calls))
		})
	})
}
