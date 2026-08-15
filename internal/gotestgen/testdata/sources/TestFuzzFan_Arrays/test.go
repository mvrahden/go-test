package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type Packet struct {
	ID    [16]byte // byte array: one []byte leaf, padded/truncated
	Grid  [3]int8  // small array: element-wise, three numeric leaves
	Big   [64]int8 // over the fan limit: one hybrid []byte leaf
	Empty struct{} // no leaves, harmless nested
}

type ArrayFuzzTestSuite struct{}

func (s *ArrayFuzzTestSuite) TestOne(t *gotest.T) {}

func (s *ArrayFuzzTestSuite) FuzzPacket(f *gotest.F) {
	f.Add(Packet{})
	gotest.Fuzz(f, func(t *gotest.T, p Packet) {
		gotest.True(t, p.Grid[0] == p.Grid[0])
	})
}
