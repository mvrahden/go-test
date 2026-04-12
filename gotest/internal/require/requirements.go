package require

import "regexp"

// testingT is the interface satisfied by *testing.T and *testing.B.
type testingT interface {
	Helper()
	Errorf(format string, args ...any)
	FailNow()
}

// regexpPattern constrains to types accepted as a regular expression.
type regexpPattern interface {
	string | *regexp.Regexp
}

// numeric constrains to Go numeric types (excluding complex).
type numeric interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr |
		~float32 | ~float64
}
