package auth

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// --- Domain types ---

var (
	ErrInvalidSignature = errors.New("invalid signature")
	ErrTokenMalformed   = errors.New("token malformed")
)

type TokenExpiredError struct{ ExpiresAt time.Time }

func (e *TokenExpiredError) Error() string {
	return fmt.Sprintf("token expired at %s", e.ExpiresAt.Format(time.RFC3339))
}

type Claims struct{ Email string }

type tokenValidator struct {
	secret string
}

func newTokenValidator(secret string) *tokenValidator {
	return &tokenValidator{secret: secret}
}

func (v *tokenValidator) Issue(email string, ttl time.Duration) string {
	return fmt.Sprintf("%s|%s|%d", v.secret, email, time.Now().Add(ttl).Unix())
}

func (v *tokenValidator) Validate(token string) (*Claims, error) {
	if len(token) < 3 {
		return nil, ErrTokenMalformed
	}
	if token[:len(v.secret)] != v.secret {
		return nil, ErrInvalidSignature
	}
	parts := splitToken(token)
	if len(parts) != 3 {
		return nil, ErrTokenMalformed
	}
	var exp int64
	fmt.Sscanf(parts[2], "%d", &exp)
	if time.Unix(exp, 0).Before(time.Now()) {
		return nil, &TokenExpiredError{ExpiresAt: time.Unix(exp, 0)}
	}
	return &Claims{Email: parts[1]}, nil
}

func splitToken(s string) []string {
	var parts []string
	start := 0
	for i := range s {
		if s[i] == '|' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func isStrongPassword(pw string) bool {
	if len(pw) < 8 {
		return false
	}
	hasDigit, hasSpecial := false, false
	for _, c := range pw {
		if c >= '0' && c <= '9' {
			hasDigit = true
		}
		if (c >= '!' && c <= '/') || (c >= ':' && c <= '@') {
			hasSpecial = true
		}
	}
	return hasDigit && hasSpecial
}

func parseConfig(data *string) string {
	if data == nil {
		panic("config: nil input")
	}
	return *data
}

var emailPattern = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

// --- Test Suites ---

type TokenValidatorTestSuite struct {
	validator *tokenValidator
}

func (s *TokenValidatorTestSuite) BeforeEach(t *gotest.T) {
	s.validator = newTokenValidator("secret-key")
}

func (s *TokenValidatorTestSuite) TestValidateToken(t *gotest.T) {
	t.When("the token is valid", func(t *gotest.T) {
		token := s.validator.Issue("user@example.com", time.Hour)
		claims, err := s.validator.Validate(token)

		t.It("returns no error", func(t *gotest.T) {
			gotest.NoError(t, err)
		})
		t.It("extracts the email claim", func(t *gotest.T) {
			gotest.Equal(t, "user@example.com", claims.Email)
		})
	})

	t.When("the token is expired", func(t *gotest.T) {
		token := s.validator.Issue("user@example.com", -time.Hour)
		_, err := s.validator.Validate(token)

		t.It("returns a TokenExpiredError", func(t *gotest.T) {
			gotest.ErrorAs[*TokenExpiredError](t, err)
		})
		t.It("includes expiry in the error message", func(t *gotest.T) {
			gotest.ErrorContains(t, err, "expired")
		})
	})

	t.When("the signature is tampered", func(t *gotest.T) {
		_, err := s.validator.Validate("wrong-key|user@x.com|9999999999")

		t.It("returns ErrInvalidSignature", func(t *gotest.T) {
			gotest.ErrorIs(t, err, ErrInvalidSignature)
		})
	})

	t.When("the token is malformed", func(t *gotest.T) {
		_, err := s.validator.Validate("ab")

		t.It("returns ErrTokenMalformed", func(t *gotest.T) {
			gotest.ErrorIs(t, err, ErrTokenMalformed)
		})
	})
}

func (s *TokenValidatorTestSuite) TestPasswordPolicy(t *gotest.T) {
	type testCase struct {
		Desc     string
		Password string
		Valid    bool
	}

	t.Each([]testCase{
		{"strong password with mixed characters", "P@ssw0rd!Long", true},
		{"too short", "P@1a", false},
		{"missing special characters", "Password123", false},
		{"missing digits", "Password!!!", false},
		{"exactly at minimum length", "A1!bcdef", true},
	}, func(t *gotest.T, tc testCase) {
		t.It("evaluates the password correctly", func(t *gotest.T) {
			gotest.Equal(t, tc.Valid, isStrongPassword(tc.Password))
		})
	})
}

func (s *TokenValidatorTestSuite) TestEmailFormat(t *gotest.T) {
	type entry struct {
		Desc    string
		Email   string
		Matches bool
	}

	for t, e := range gotest.Each(t, []entry{
		{"standard email", "alice@example.com", true},
		{"missing at sign", "alice-example.com", false},
		{"subdomain email", "bob@mail.example.co.uk", true},
		{"missing TLD", "carol@localhost", false},
	}) {
		t.When("checking the format", func(t *gotest.T) {
			matched := emailPattern.MatchString(e.Email)

			t.It("matches as expected", func(t *gotest.T) {
				gotest.Equal(t, e.Matches, matched)
			})
			if e.Matches {
				t.It("passes the full regex", func(t *gotest.T) {
					gotest.Regexp(t, emailPattern, e.Email)
				})
			}
		})
	}
}

func (s *TokenValidatorTestSuite) TestParseConfig(t *gotest.T) {
	t.When("the config string is nil", func(t *gotest.T) {
		t.It("panics with a clear message", func(t *gotest.T) {
			recovered := gotest.Panics(t, func() { parseConfig(nil) })
			gotest.Contains(t, recovered, "nil input")
		})
	})

	t.When("the config string is provided", func(t *gotest.T) {
		input := "host=localhost"
		result := gotest.Must(parseConfig(&input), nil)

		t.It("returns the parsed value", func(t *gotest.T) {
			gotest.Equal(t, "host=localhost", result)
		})
	})
}

// F_TokenValidatorTestSuite demonstrates a focused suite. When any F_ suite
// exists, only focused suites run; unfocused suites are skipped.
type F_TokenValidatorTestSuite = TokenValidatorTestSuite

// X_DeprecatedOAuthTestSuite demonstrates an excluded suite. It is always
// skipped regardless of focus state.
type X_DeprecatedOAuthTestSuite struct{}

func (s *X_DeprecatedOAuthTestSuite) TestLegacyFlow(t *gotest.T) {
	t.When("using the deprecated OAuth flow", func(t *gotest.T) {
		t.It("would fail if it ran", func(t *gotest.T) {
			gotest.Fail(t, "this suite is excluded and should never run")
		})
	})
}
