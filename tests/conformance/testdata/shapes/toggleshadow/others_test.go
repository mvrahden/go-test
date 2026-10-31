package toggleshadow

func local() int {
	ShadowTestSuite := 1
	return ShadowTestSuite
}

type field struct{ ShadowTestSuite int }

type method struct{}

func (method) ShadowTestSuite() int { return field{ShadowTestSuite: 1}.ShadowTestSuite }

var _ = method.ShadowTestSuite
