package gotest

import "time"

type R struct{}

func (r *R) Errorf(string, ...any) {}
func (r *R) FailNow()              {}
func (r *R) Helper()               {}
func (r *R) Failed() bool          { return false }
func (r *R) Message() string       { return "" }

type T struct{}

func (t *T) Errorf(string, ...any) {}
func (t *T) FailNow()              {}

type testingT interface {
	Errorf(format string, args ...any)
	FailNow()
}

func Eventually(t testingT, waitFor, tick time.Duration, fn func(poll *R))    {}
func Consistently(t testingT, waitFor, tick time.Duration, fn func(poll *R))  {}
func Equal(t testingT, expected, actual any, msgAndArgs ...any)               {}
func True(t testingT, value bool, msgAndArgs ...any)                          {}
func NoError(t testingT, err error, msgAndArgs ...any)                        {}
func False(t testingT, value bool, msgAndArgs ...any)                         {}
func MatchSnapshot(t testingT, value any, name ...string)                     {}
