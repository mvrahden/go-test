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
	{file: "reference/cli.md", quote: "`gotest clean --cache`", label: "v1.38+", path: "cmd/gotest/clean.go", literal: "--cache"},
	{file: "SKILL.md", quote: "the run refuses it", label: "v1.38+", path: "internal/gotestast/orphans.go", literal: "uninstantiated refuses a generic suite"},
	{file: "SKILL.md", quote: "a method named just `Test`", label: "v1.38+", path: "internal/gotestast/spec.go", literal: "without a name after the prefix"},
	{file: "SKILL.md", quote: "uses the fixture (v1.38+)", label: "v1.38+", path: "internal/gotestast/parallel.go", literal: "which the suite's parallel methods share"},
	{file: "SKILL.md", quote: "`BeforeAll` may call it", label: "v1.38+", path: "internal/gotestast/parallel.go", literal: "testing panics when a parallel test sets the environment"},
	{file: "reference/api.md", quote: "JSON text must be valid UTF-8", label: "v1.38+", path: "pkg/gotest/internal/assert/predicates.go", literal: "invalid UTF-8 at byte"},
	{file: "reference/api.md", quote: "no test read", label: "v1.38+", path: "specs/assertions/spec.md", literal: "### Requirement: Sections no test read"},
	{file: "reference/api.md", quote: "fail on a NaN operand", label: "v1.38+", path: "specs/assertions/spec.md", literal: "### Requirement: Ordering a NaN"},
	{file: "reference/api.md", quote: "fail on a NaN operand", label: "v1.38+", path: "pkg/gotest/assertions.go", literal: "points to a value that cannot be empty"},
	{file: "reference/api.md", quote: "the wait is held to the method's declared", label: "v1.38+", path: "specs/lifecycle/spec.md", literal: "the wait SHALL be held to the method's declared Timeout"},
	{file: "reference/api.md", quote: "The converse is refused too", label: "v1.38+", path: "internal/gotestast/orphans.go", literal: "is not a test suite (its name must end in TestSuite)"},
	{file: "reference/api.md", quote: "is refused at the method", label: "v1.38+", path: "internal/gotestast/spec.go", literal: "without a name after the prefix"},
	{file: "reference/api.md", quote: "ahead of one stack", label: "v1.38+", path: "specs/lifecycle/spec.md", literal: "### Requirement: A method's panic is printed once"},
	{file: "reference/api.md", quote: "It also refuses (v1.38+)", label: "v1.38+", path: "internal/gotestast/parallel.go", literal: "which the suite's parallel methods share"},
	{file: "reference/api.md", quote: "has no deadline and ends when the methods are done", label: "v1.38+", path: "specs/lifecycle/spec.md", literal: "### Requirement: What BeforeAll takes from its T"},
	{file: "reference/api.md", quote: "naming the fixture, the hook and the error", label: "v1.38+", path: "specs/fixtures/spec.md", literal: "### Requirement: What a failed fixture BeforeAll or AfterAll says"},
	{file: "reference/api.md", quote: "a context a stop does not end", label: "v1.38+", path: "specs/fixtures/spec.md", literal: "### Requirement: The context a fixture's AfterEach gets"},
	{file: "reference/api.md", quote: "that uses the fixture is refused", label: "v1.38+", path: "internal/gotestast/parallel.go", literal: "which the suite's parallel methods share"},
	{file: "reference/api.md", quote: "Only named exported fields are transferred", label: "v1.38+", path: "internal/gotestast/transfer.go", literal: "an interface the state cannot carry"},
	{file: "reference/api.md", quote: "an `X_` one aside", label: "v1.38+", path: "internal/gotestast/orphans.go", literal: "uninstantiated refuses a generic suite"},
	{file: "reference/api.md", quote: "lasts until `AfterAll` has returned", label: "v1.38+", path: "specs/lifecycle/spec.md", literal: "### Requirement: What BeforeAll takes from its T"},
	{file: "reference/api.md", quote: "lasts until `AfterAll` has returned", label: "v1.38+", path: "internal/gotestast/parallel.go", literal: "testing panics when a parallel test sets the environment"},
	{file: "reference/cli.md", quote: "and so are (v1.38+)", label: "v1.38+", path: "cmd/gotest/cli.go", literal: "-overlay is not supported"},
	{file: "reference/cli.md", quote: "replays none of its fuzz seeds", label: "v1.38+", path: "internal/gotestrunner/suiterun.go", literal: "func processRunFilter"},
	{file: "reference/cli.md", quote: "(v1.38+), and a text run", label: "v1.38+", path: "specs/cli/spec.md", literal: "### Requirement: A focused run outside CI mode"},
	{file: "reference/cli.md", quote: "tests (v1.38+).", label: "v1.38+", path: "specs/cli/spec.md", literal: "### Requirement: A suite that ran no tests"},
	{file: "reference/cli.md", quote: "and without it the run's own", label: "v1.38+", path: "cmd/gotest/help.go", literal: "gotest spec --declared [flags] [packages...]"},
	{file: "reference/cli.md", quote: "message (rule)", label: "v1.38+", path: "cmd/gotest/help.go", literal: "Each finding line ends with its rule"},
	{file: "reference/cli.md", quote: "`-nolint-allow` names other linters", label: "v1.38+", path: "cmd/gotest/help.go", literal: "nolint-allow"},
	{file: "reference/cli.md", quote: "at the gotest version", label: "v1.38+", path: "pkg/lint/golangci/plugin.go", literal: "package golangci"},
	{file: "reference/cli.md", quote: "running (v1.38+)", label: "v1.38+", path: "specs/cli/spec.md", literal: "### Requirement: A suite cut short by --timeout"},
	{file: "reference/config.md", quote: "before `AfterAll` (v1.38+)", label: "v1.38+", path: "specs/lifecycle/spec.md", literal: "### Requirement: What BeforeAll takes from its T"},
	{file: "reference/config.md", quote: "fails the run with exit 2", label: "v1.38+", path: "cmd/gotest/help.go", literal: "gotest fuzz ignores timeout and setup-timeout"},
	{file: "reference/fixtures.md", quote: "when the hook uses the fixture", label: "v1.38+", path: "internal/gotestast/parallel.go", literal: "which the suite's parallel methods share"},
	{file: "reference/fixtures.md", quote: "the suite declares (v1.38+)", label: "v1.38+", path: "specs/fixtures/spec.md", literal: "### Requirement: The context a fixture's AfterEach gets"},
	{file: "reference/fixtures.md", quote: "the hook and the error (v1.38+)", label: "v1.38+", path: "specs/fixtures/spec.md", literal: "### Requirement: What a failed fixture BeforeAll or AfterAll says"},
	{file: "reference/fixtures.md", quote: "refuses (v1.38+) an embedded", label: "v1.38+", path: "internal/gotestast/transfer.go", literal: "an interface the state cannot carry"},
	{file: "reference/cli.md", quote: "incl. every subtest name", label: "v1.42+", path: "internal/protocol/manifest.go", literal: "RunPackage"},
	{file: "SKILL.md", quote: "Write the reason", label: "v1.42+", path: "internal/lint/lint.go", literal: "nolint-reason"},
	{file: "SKILL.md", quote: "audit it against its base", label: "v1.42+", path: "cmd/gotest/audit.go", literal: "func runAudit"},
	{file: "reference/cli.md", quote: "go tool gotest audit --base=<ref>", label: "v1.42+", path: "cmd/gotest/audit.go", literal: "--head-record"},
	{file: "SKILL.md", quote: "bind each one you implement by its text", label: "v1.42+", path: "cmd/gotest/scenarios.go", literal: "func runScenarios"},
	{file: "reference/ci.md", quote: "audit and attestation inputs", label: "v1.42+", path: "action.yml", literal: "audit-fail-on"},
	{file: "reference/cli.md", quote: "prints the saved run's result", label: "v1.42+", path: "cmd/gotest/attest.go", literal: "func runTestResult"},
	{file: "reference/cli.md", quote: "go tool gotest scenarios", label: "v1.42+", path: "cmd/gotest/scenarios.go", literal: "--unstated"},
	{file: "reference/cli.md", quote: "its reason, as JSON", label: "v1.42+", path: "cmd/gotest/lint.go", literal: "suppressions"},
	{file: "reference/api.md", quote: "except on an `Each` row's `T`", label: "v1.42+", path: "pkg/gotest/utils.go", literal: "row.soft = true"},
	{file: "reference/assertions.md", quote: "An `Each` row's `t` is the exception", label: "v1.42+", path: "pkg/gotest/utils.go", literal: "row.soft = true"},
	{file: "reference/assertions.md", quote: "a failing row goes on and so do the rows after it", label: "v1.42+", path: "pkg/gotest/utils.go", literal: "row.soft = true"},
	{file: "SKILL.md", quote: "only what failed, then one closing line", label: "v1.42+", path: "internal/gotestrunner/agentoutput.go", literal: "func NewAgentOutput"},
	{file: "reference/cli.md", quote: "A text run ends with one closing line", label: "v1.42+", path: "internal/gotestrunner/agentoutput.go", literal: "func (c Closing) String() string"},
	{file: "reference/cli.md", quote: "fails a text run with exit 2", label: "v1.42+", path: "internal/gotestrunner/census.go", literal: "func reportsResult"},
	{file: "reference/cli.md", quote: "there is no record or nothing to rerun", label: "v1.42+", path: "cmd/gotest/flags.go", literal: "--failed"},
	{file: "SKILL.md", quote: "go tool gotest --changed ./...", label: "v1.42+", path: "cmd/gotest/flags.go", literal: "--changed"},
	{file: "reference/cli.md", quote: "`--changed` (v1.42+)", label: "v1.42+", path: "cmd/gotest/changed.go", literal: "func planRun"},
	{file: "reference/cli.md", quote: "The plain text run is censused too", label: "v1.42+", path: "internal/gotestrunner/census.go", literal: "func reportsResult"},
	{file: "reference/reporting.md", quote: "text run (v1.42+", label: "v1.42+", path: "internal/gotestrunner/census.go", literal: "func reportsResult"},
	{file: "SKILL.md", quote: "asserts, or says how it checks (v1.42+)", label: "v1.42+", path: "pkg/gotest/census.go", literal: "func (t *T) AssertsBy(reason string)"},
	{file: "reference/api.md", quote: "### A test asserts, or says how it checks", label: "v1.42+", path: "pkg/gotest/census.go", literal: "func (t *T) AssertsBy(reason string)"},
	{file: "reference/api.md", quote: "the test checks by other means; never to silence the count", label: "v1.42+", path: "pkg/gotest/census.go", literal: "func (t *T) AssertsBy(reason string)"},
	{file: "reference/ci.md", quote: "an assertion fails it (v1.42+", label: "v1.42+", path: "pkg/gotest/census.go", literal: "func (t *T) AssertsBy(reason string)"},
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
	out, err := exec.CommandContext(t.Context(), "git", "-C", repoRoot(t), "tag", "--list", "v[0-9]*", "--sort=-v:refname").Output() //nolint:gosec // G204: git on the repository root
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
	out, err := exec.CommandContext(t.Context(), "git", "-C", repoRoot(t), "show", tag+":"+path).Output() //nolint:gosec // G204: tags and paths of this repository
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
