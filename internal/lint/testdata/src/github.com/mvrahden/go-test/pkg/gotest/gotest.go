package gotest

import (
	"testing"
	"time"
)

type R struct{}

func (r *R) Errorf(string, ...any) {}
func (r *R) FailNow()              {}
func (r *R) Helper()               {}
func (r *R) Failed() bool          { return false }
func (r *R) Message() string       { return "" }

type T struct{}

func (t *T) Errorf(string, ...any) {}
func (t *T) FailNow()              {}
func (t *T) Skipf(string, ...any)  {}
func (t *T) Setenv(string, string) {}
func (t *T) TempDir() string       { return "" }
func (t *T) T() *testing.T         { return nil }
func (t *T) It(string, func(*T))   {}
func (t *T) When(string, func(*T)) {}

type testingT interface {
	Errorf(format string, args ...any)
	FailNow()
}

func Eventually(t testingT, waitFor, tick time.Duration, fn func(poll *R))   {}
func Consistently(t testingT, waitFor, tick time.Duration, fn func(poll *R)) {}

func True(t testingT, value bool, msgAndArgs ...any)                          {}
func False(t testingT, value bool, msgAndArgs ...any)                         {}
func Equal(t testingT, expected, actual any, msgAndArgs ...any)               {}
func NotEqual(t testingT, expected, actual any, msgAndArgs ...any)            {}
func Greater(t testingT, a, b any, msgAndArgs ...any)                         {}
func GreaterOrEqual(t testingT, a, b any, msgAndArgs ...any)                  {}
func Less(t testingT, a, b any, msgAndArgs ...any)                            {}
func LessOrEqual(t testingT, a, b any, msgAndArgs ...any)                     {}
func Zero(t testingT, value any, msgAndArgs ...any)                           {}
func NotZero(t testingT, value any, msgAndArgs ...any)                        {}
func Empty(t testingT, object any, msgAndArgs ...any)                         {}
func NotEmpty(t testingT, object any, msgAndArgs ...any)                      {}
func Nil(t testingT, object any, msgAndArgs ...any)                           {}
func NotNil(t testingT, object any, msgAndArgs ...any)                        {}
func Len(t testingT, object any, length int, msgAndArgs ...any)               {}
func Contains(t testingT, s, contains any, msgAndArgs ...any)                 {}
func NotContains(t testingT, s, contains any, msgAndArgs ...any)              {}
func NoError(t testingT, err error, msgAndArgs ...any)                        {}
func Error(t testingT, err error, msgAndArgs ...any)                          {}
func ErrorIs(t testingT, err, target error, msgAndArgs ...any)                {}
func ErrorContains(t testingT, err error, contains string, msgAndArgs ...any) {}
func Regexp(t testingT, rx, str any, msgAndArgs ...any)                       {}
func MatchSnapshot(t testingT, value any, name ...string)                     {}
