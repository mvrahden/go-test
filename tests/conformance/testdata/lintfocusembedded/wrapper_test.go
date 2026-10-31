package lintfocusembedded

type wrapper struct{ F_BaseTestSuite }

func (w wrapper) count() int { return w.F_BaseTestSuite.n }

var _ = wrapper{F_BaseTestSuite: F_BaseTestSuite{n: 1}}.count
