package conventions_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ActionInputsTestSuite pins action.yml to the splitting idiom that honours
// newlines. A bare `read -ra x <<<` keeps only the first line of a block
// scalar, which once cut a consumer's package list to its first entry while
// the job stayed green. GOTEST_CMD is one line by construction and may keep
// the bare form.
type ActionInputsTestSuite struct{}

func (s *ActionInputsTestSuite) TestInputsAreSplitOnEveryLine(t *gotest.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	gotest.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(root, "action.yml"))
	gotest.NoError(t, err)

	var bare []string
	for i, line := range strings.Split(string(data), "\n") {
		if !strings.Contains(line, "read -ra ") || strings.Contains(line, "GOTEST_CMD") {
			continue
		}
		if !strings.Contains(line, "-d ''") {
			bare = append(bare, "action.yml:"+strconv.Itoa(i+1)+": "+strings.TrimSpace(line))
		}
	}
	gotest.Empty(t, bare, "these reads stop at the first newline; use `read -ra name -d '' <<< \"$VALUE\" || true`")
}
