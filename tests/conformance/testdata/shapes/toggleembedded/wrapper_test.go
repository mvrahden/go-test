package toggleembedded

type wrapper struct{ BaseTestSuite }

func (w wrapper) count() int { return w.BaseTestSuite.n }

var _ = wrapper{BaseTestSuite: BaseTestSuite{n: 1}}.count
