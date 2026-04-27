package about

import "regexp"

const (
	PSuite  = "ƒƒ_psuite_test.go"
	PXSuite = "ƒƒ_pxsuite_test.go"
)

var PSuiteRegex = regexp.MustCompile(`ƒƒ_p(x)?suite_test\.go$`)

const (
	Application = "gotest"
	Repo        = "github.com/mvrahden/go-test"
)

var Version = "dev"
