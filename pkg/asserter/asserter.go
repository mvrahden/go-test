package asserter

import "testing"

func NewAsserter(t *testing.T) Asserter {
	return Asserter{t}
}

type Asserter struct {
	t *testing.T
}

func (Asserter) Not() BaseNegator {
	return BaseNegator{}
}

func (Asserter) Any() AnyAsserter {
	return AnyAsserter{}
}

func (Asserter) Time() TimeAsserter {
	return TimeAsserter{}
}

func (Asserter) Duration() DurationAsserter {
	return DurationAsserter{}
}

type AnyAsserter struct {
	Not Negator
}

type BaseNegator struct {
	t *testing.T
}

type Negator struct{}
