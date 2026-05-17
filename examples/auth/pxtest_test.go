package auth_test

import "github.com/mvrahden/go-test/pkg/gotest"

type TokenValidatorTestSuite struct {
	secret string
}

func (s *TokenValidatorTestSuite) BeforeEach(t *gotest.T) {
	s.secret = "ext-secret"
}

func (s *TokenValidatorTestSuite) TestSecretIsConfigured(t *gotest.T) {
	t.When("the validator is initialized", func(t *gotest.T) {
		t.It("has a non-empty secret", func(t *gotest.T) {
			gotest.NotEmpty(t, s.secret)
		})
	})
}
