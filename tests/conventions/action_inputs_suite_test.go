package conventions_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ActionInputsTestSuite pins action.yml to the splitting idiom that honours
// newlines. A bare `read -ra x <<<` keeps only the first line of a block
// scalar, which once cut a consumer's package list to its first entry while
// the job stayed green.
type ActionInputsTestSuite struct{}

// arrayRead is a read into an array: an option cluster after read holds a.
var arrayRead = regexp.MustCompile(`\bread(\s+-[A-Za-z]+(\s+'[^']*'|\s+"[^"]*")?)*\s+-[A-Za-z]*a[A-Za-z]*\s`)

func (s *ActionInputsTestSuite) TestInputsAreSplitOnEveryLine(t *gotest.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	gotest.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(root, "action.yml"))
	gotest.NoError(t, err)

	var reads, bare []string
	for i, line := range strings.Split(string(data), "\n") {
		if !arrayRead.MatchString(line) {
			continue
		}
		reads = append(reads, line)
		if !strings.Contains(line, "-d ''") {
			bare = append(bare, "action.yml:"+strconv.Itoa(i+1)+": "+strings.TrimSpace(line))
		}
	}
	gotest.NotEmpty(t, reads, "action.yml splits its list inputs with read -a")
	gotest.Empty(t, bare, "these reads stop at the first newline; use `read -r -d '' -a name <<< \"$VALUE\" || true`")
}
