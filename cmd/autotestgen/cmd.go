package autotestgen

import (
	"github.com/mvrahden/go-test/internal/cmd/testgen"
)

func Generate() error {
	return testgen.Execute(nil)
}
