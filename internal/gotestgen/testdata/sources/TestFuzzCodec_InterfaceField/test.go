package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type WithAny struct {
	Payload any
}

type InterfaceFuzzTestSuite struct{}

func (s *InterfaceFuzzTestSuite) TestOne(t *gotest.T) {}

func (s *InterfaceFuzzTestSuite) FuzzAny(f *gotest.F) {
	gotest.Fuzz(f, func(t *gotest.T, v WithAny) {
		gotest.True(t, v.Payload == v.Payload)
	})
}
