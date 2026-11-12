package skillevals_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// SkillLabelsTestSuite: a "vN.M+" label in the skill names the release
// that first carries the feature, judged against the local release tags.
type SkillLabelsTestSuite struct{}

func (s *SkillLabelsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

// SuiteGuard: a checkout without tags (a shallow CI clone) cannot judge a label.
func (s *SkillLabelsTestSuite) SuiteGuard() string {
	if _, err := exec.LookPath("git"); err != nil {
		return "git is not on PATH"
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		return err.Error()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", root, "tag", "--list", "v[0-9]*").Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		return "this checkout has no release tags; fetch them with git fetch --tags"
	}
	return ""
}

// witness ties a labelled line of the skill to a literal that exists in a
// file of the repository from the labelled release on.
type witness struct {
	file    string // the skill file, from the skill's directory
	quote   string // text on the labelled line
	label   string // the label on that line, such as "v1.37+"
	path    string // the repository file that carries the feature
	literal string // text that file holds from the labelled release on
}

var witnesses = []witness{
	{file: "SKILL.md", quote: "name helper types otherwise", label: "v1.37+", path: "internal/gotestast/shapes.go", literal: "declares no Test, Benchmark or Fuzz method"},
	{file: "SKILL.md", quote: "`t-escape` flags the rest", label: "v1.37+", path: "pkg/gotest/b.go", literal: "func (b *B) Setenv"},
	{file: "reference/cli.md", quote: "`--control=stdin`", label: "v1.37+", path: "cmd/gotest/flags.go", literal: "--control"},
	{file: "reference/migration.md", quote: "never writes", label: "v1.37+", path: "specs/migrate/spec.md", literal: "### Requirement: Module files"},
	{file: "reference/ci.md", quote: "action's run as Ctrl-C does", label: "v1.37+", path: "action.yml", literal: "trap 'kill -INT"},
	{file: "reference/ci.md", quote: "also stops on", label: "v1.37+", path: "cmd/gotest/signals_unix.go", literal: "SIGHUP"},
}

var (
	labelToken = regexp.MustCompile(`\bv(\d+)\.(\d+)(?:\.(\d+))?\+`)
	describes  = regexp.MustCompile(`This skill describes \*\*(v\d+\.\d+)\*\*`)
	tagVersion = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)$`)
)

// labelLines are lines that explain labels rather than carry one.
var labelLines = []string{"A rule tagged", "Rules and sections tagged"}

// version is a release's major.minor.patch.
type version [3]int

func (v version) less(o version) bool {
	for i := range v {
		if v[i] != o[i] {
			return v[i] < o[i]
		}
	}
	return false
}

// parseVersion reads "v1.37.0", "v1.37+" or "v1.37"; a missing patch is 0.
func parseVersion(s string) (version, bool) {
	m := regexp.MustCompile(`^v(\d+)\.(\d+)(?:\.(\d+))?\+?$`).FindStringSubmatch(s)
	if m == nil {
		return version{}, false
	}
	var v version
	for i := range 3 {
		if m[i+1] != "" {
			v[i], _ = strconv.Atoi(m[i+1])
		}
	}
	return v, true
}

// releaseTags lists the release tags, newest first.
func releaseTags(t *gotest.T) []string {
	out, err := exec.CommandContext(t.Context(), "git", "-C", repoRoot(t), "tag", "--list", "v[0-9]*", "--sort=-v:refname").Output()
	gotest.NoError(t, err)
	var tags []string
	for _, tag := range strings.Fields(string(out)) {
		if tagVersion.MatchString(tag) {
			tags = append(tags, tag)
		}
	}
	gotest.NotEmpty(t, tags, "no release tag of the form vN.M.P")
	return tags
}

// tagBelow is the newest tag that sorts below release, "" when none.
func tagBelow(tags []string, release string) string {
	rel, _ := parseVersion(release)
	for _, tag := range tags {
		if v, _ := parseVersion(tag); v.less(rel) {
			return tag
		}
	}
	return ""
}

// firstTagOf is the oldest tag at or above release within its minor, "" when none.
func firstTagOf(tags []string, release string) string {
	rel, _ := parseVersion(release)
	first := ""
	for _, tag := range tags {
		v, _ := parseVersion(tag)
		if v[0] == rel[0] && v[1] == rel[1] && !v.less(rel) {
			first = tag // tags are newest first, so the last match is the oldest
		}
	}
	return first
}

// atTag reads path at tag; a path the tag does not have is "", false.
func atTag(t *gotest.T, tag, path string) (string, bool) {
	out, err := exec.CommandContext(t.Context(), "git", "-C", repoRoot(t), "show", tag+":"+path).Output()
	if err != nil {
		return "", false
	}
	return string(out), true
}

func (s *SkillLabelsTestSuite) TestALabelIsNoNewerThanItsFeature(t *gotest.T) {
	tags := releaseTags(t)
	files := map[string][]string{}
	for _, f := range skillFiles(t) {
		files[f.rel] = f.lines
	}
	var wrong []string
	for _, w := range witnesses {
		onLine := false
		for _, line := range files[w.file] {
			if strings.Contains(line, w.quote) && strings.Contains(line, w.label) {
				onLine = true
				break
			}
		}
		if !onLine {
			wrong = append(wrong, fmt.Sprintf("%s: no line holds %q with the label %s; update the witness", w.file, w.quote, w.label))
			continue
		}
		if below := tagBelow(tags, w.label); below != "" {
			if text, ok := atTag(t, below, w.path); ok && strings.Contains(text, w.literal) {
				wrong = append(wrong, fmt.Sprintf("%s: %q is labelled %s, but %s already has %q in %s; label it with the release that first had it", w.file, w.quote, w.label, below, w.literal, w.path))
				continue
			}
		}
		where, text, ok := firstTagOf(tags, w.label), "", false
		if where != "" {
			text, ok = atTag(t, where, w.path)
		} else {
			where = "the working tree"
			data, err := os.ReadFile(filepath.Join(repoRoot(t), filepath.FromSlash(w.path))) //nolint:gosec // G304: the repository's own files
			text, ok = string(data), err == nil
		}
		if !ok || !strings.Contains(text, w.literal) {
			wrong = append(wrong, fmt.Sprintf("%s: %q is labelled %s, but %s holds no %q; fix the label or the witness", w.file, w.quote, w.label, where, w.literal))
		}
	}
	t.When("a labelled rule is traced to the code that carries it", func(w *gotest.T) {
		w.It("the release before the label does not have it and the labelled release does", func(it *gotest.T) {
			gotest.Empty(it, wrong, "%s", strings.Join(wrong, "\n"))
		})
	})
}

func (s *SkillLabelsTestSuite) TestNewLabelsHaveAWitness(t *gotest.T) {
	newest, _ := parseVersion(releaseTags(t)[0])
	var unwitnessed []string
	for _, f := range skillFiles(t) {
	lines:
		for i, line := range f.lines {
			for _, skip := range labelLines {
				if strings.Contains(line, skip) {
					continue lines
				}
			}
			for _, m := range labelToken.FindAllStringSubmatch(line, -1) {
				v, _ := parseVersion(strings.TrimSuffix(m[0], "+"))
				if v[0] < newest[0] || (v[0] == newest[0] && v[1] < newest[1]) {
					continue
				}
				if !hasWitness(f.rel, line, m[0]) {
					unwitnessed = append(unwitnessed, fmt.Sprintf("%s:%d: %s", f.rel, i+1, m[0]))
				}
			}
		}
	}
	t.When("the skill labels a rule with the newest release or the next", func(w *gotest.T) {
		w.It("a witness row names the code that carries it", func(it *gotest.T) {
			gotest.Empty(it, unwitnessed, "add a row to witnesses in tests/skillevals/labels_suite_test.go:\n%s", strings.Join(unwitnessed, "\n"))
		})
	})
}

// hasWitness reports whether a witness row covers the label on this line.
func hasWitness(file, line, label string) bool {
	for _, w := range witnesses {
		if w.file == file && w.label == label && strings.Contains(line, w.quote) {
			return true
		}
	}
	return false
}

func (s *SkillLabelsTestSuite) TestTheSkillDescribesTheNewestRelease(t *gotest.T) {
	newestTag := releaseTags(t)[0]
	v, _ := parseVersion(newestTag)
	newest := fmt.Sprintf("v%d.%d", v[0], v[1])
	next := fmt.Sprintf("v%d.%d", v[0], v[1]+1)
	described := ""
	for _, f := range skillFiles(t) {
		if f.rel != "SKILL.md" {
			continue
		}
		if m := describes.FindStringSubmatch(strings.Join(f.lines, "\n")); m != nil {
			described = m[1]
		}
	}
	t.When("SKILL.md says which release it describes", func(w *gotest.T) {
		w.It("names the newest release tag's minor, or the next while a release commit is untagged", func(it *gotest.T) {
			gotest.Contains(it, []string{newest, next}, described,
				"SKILL.md describes %s; the newest tag is %s. Main's skill describes the released version; the release commit bumps it", described, newestTag)
		})
	})
}
