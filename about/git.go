package about

import (
	"os/exec"
	"regexp"
	"strings"
)

const (
	PSuite  = "ƒƒ_psuite_test.go"
	PXSuite = "ƒƒ_pxsuite_test.go"
)

var PSuiteRegex = regexp.MustCompile(`ƒƒ_p(x)?suite_test\.go$`)

const (
	Application = "go-test"
	Repo        = "github.com/mvrahden/go-test"
)

var (
	GitFormat = "local"
	GitCommit = "local"
	GitBranch = "local"
	GitTag    = "local"
)

func init() {
	format := exec.Command("git", "log", "--pretty=format:git info=[%h,%d]", "-n", "1")
	commit := exec.Command("git", "rev-list", "-1", "HEAD")
	branch := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	tag := exec.Command("git", "describe", "--tags")
	if data, err := format.Output(); err == nil {
		GitFormat = strings.TrimSpace(string(data))
	}
	if data, err := commit.Output(); err == nil {
		GitCommit = strings.TrimSpace(string(data))
	}
	if data, err := branch.Output(); err == nil {
		GitBranch = strings.TrimSpace(string(data))
	}
	if data, err := tag.Output(); err == nil {
		GitTag = strings.TrimSpace(string(data))
	}
}
