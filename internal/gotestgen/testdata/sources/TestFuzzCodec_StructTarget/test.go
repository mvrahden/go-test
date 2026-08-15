package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type Priority int

type Address struct {
	Street string
	Zip    uint16
}

type Request struct {
	Email string
	Age   int
	Prio  Priority
	Tags  []string
	Home  *Address
}

type StructFuzzTestSuite struct{}

func (s *StructFuzzTestSuite) TestOne(t *gotest.T) {}

func (s *StructFuzzTestSuite) FuzzCreate(f *gotest.F) {
	f.Add(Request{Email: "a@b.c", Age: 30})
	gotest.Fuzz(f, func(t *gotest.T, req Request) {
		gotest.True(t, req.Age == req.Age)
	})
}

// FuzzNative stays on the native path — its codec list must not grow one.
func (s *StructFuzzTestSuite) FuzzNative(f *gotest.F) {
	f.Add("x")
	gotest.Fuzz(f, func(t *gotest.T, in string) {
		gotest.True(t, in == in)
	})
}
