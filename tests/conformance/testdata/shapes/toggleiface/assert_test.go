package toggleiface

import "github.com/mvrahden/go-test/pkg/gotest"

var _ interface{ TestA(*gotest.T) } = (*IfaceTestSuite)(nil)
