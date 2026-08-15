package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type PtrReq struct {
	Count *int
}

type PtrTestSuite struct{}

func (s *PtrTestSuite) FuzzPtr(f *gotest.F) {
	gotest.Fuzz(f, func(t *gotest.T, r PtrReq) { _ = r })
}
