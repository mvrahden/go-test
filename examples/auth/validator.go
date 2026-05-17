package auth

import (
	"errors"
	"fmt"
	"regexp"
	"time"
)

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
