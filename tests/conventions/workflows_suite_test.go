package conventions_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// WorkflowsTestSuite holds the workflows to the invariants a reviewer would
// otherwise have to remember: timeouts, pinned actions, unpinned npx tools,
// and trigger paths that agree with the release preflight.
type WorkflowsTestSuite struct{}

func (s *WorkflowsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

type workflowFile struct {
	On   map[string]yaml.Node   `yaml:"on"`
	Jobs map[string]workflowJob `yaml:"jobs"`
}

type workflowJob struct {
	Uses           string         `yaml:"uses"`
	TimeoutMinutes int            `yaml:"timeout-minutes"`
	ContinueOnErr  yaml.Node      `yaml:"continue-on-error"`
	Steps          []workflowStep `yaml:"steps"`
}

type workflowStep struct {
	ID, Name, Uses, Run, If string
	With, Env               map[string]string
	ContinueOnErr           yaml.Node `yaml:"continue-on-error"`
}

var workflowsDir = filepath.Join("..", "..", ".github", "workflows")

// readWorkflow parses .github/workflows/<name>.
func readWorkflow(t *gotest.T, name string) workflowFile {
	data, err := os.ReadFile(filepath.Join(workflowsDir, name))
	gotest.NoError(t, err)
	var wf workflowFile
	gotest.NoError(t, yaml.Unmarshal(data, &wf), "%s", name)
	return wf
}

// readWorkflows parses every workflow, keyed by file name.
func readWorkflows(t *gotest.T) map[string]workflowFile {
	files, err := filepath.Glob(filepath.Join(workflowsDir, "*.yml"))
	gotest.NoError(t, err)
	gotest.NotEmpty(t, files)
	all := map[string]workflowFile{}
	for _, f := range files {
		all[filepath.Base(f)] = readWorkflow(t, filepath.Base(f))
	}
	return all
}

// eventPaths is the paths list of one trigger of a workflow, nil when it has none.
func eventPaths(t *gotest.T, wf workflowFile, event string) []string {
	node, ok := wf.On[event]
	if !ok {
		return nil
	}
	var trigger struct {
		Paths []string `yaml:"paths"`
	}
	if node.Kind == yaml.MappingNode {
		gotest.NoError(t, node.Decode(&trigger), "the %s trigger", event)
	}
	return trigger.Paths
}

func (s *WorkflowsTestSuite) TestEveryJobHasATimeout(t *gotest.T) {
	var missing []string
	for file, wf := range readWorkflows(t) {
		for name, job := range wf.Jobs {
			if job.Uses == "" && job.TimeoutMinutes == 0 {
				missing = append(missing, file+": "+name)
			}
		}
	}
	sort.Strings(missing)
	gotest.Empty(t, missing, "jobs without timeout-minutes")
}

var pinned = regexp.MustCompile(`^[^@]+@[0-9a-f]{40}$`)

func (s *WorkflowsTestSuite) TestActionsArePinnedByCommit(t *gotest.T) {
	var uses []string
	for file, wf := range readWorkflows(t) {
		for name, job := range wf.Jobs {
			for _, st := range job.Steps {
				if st.Uses != "" {
					uses = append(uses, file+": "+name+": "+st.Uses)
				}
			}
		}
	}
	actions, err := filepath.Glob(filepath.Join("..", "..", ".github", "actions", "*", "action.yml"))
	gotest.NoError(t, err)
	for _, file := range append([]string{filepath.Join("..", "..", "action.yml")}, actions...) {
		data, err := os.ReadFile(file)
		gotest.NoError(t, err)
		var action struct {
			Runs struct {
				Steps []workflowStep `yaml:"steps"`
			} `yaml:"runs"`
		}
		gotest.NoError(t, yaml.Unmarshal(data, &action), "%s", file)
		for _, st := range action.Runs.Steps {
			if st.Uses != "" {
				uses = append(uses, filepath.ToSlash(file)+": "+st.Uses)
			}
		}
	}
	gotest.NotEmpty(t, uses)
	var loose []string
	for _, u := range uses {
		ref := u[strings.LastIndex(u, ": ")+2:]
		if !strings.HasPrefix(ref, "./") && !pinned.MatchString(ref) {
			loose = append(loose, u)
		}
	}
	sort.Strings(loose)
	gotest.Empty(t, loose, "actions not pinned by a 40-hex commit")
}

var pinnedNpx = regexp.MustCompile(`npx\s+(-\S+\s+)*(@[\w.-]+/)?[\w.-]+@\d`)

func (s *WorkflowsTestSuite) TestNpxToolsAreNotPinned(t *gotest.T) {
	root := filepath.Join("..", "..")
	files := []string{filepath.Join(root, "action.yml"), filepath.Join(root, "Makefile")}
	gotest.NoError(t, filepath.WalkDir(filepath.Join(root, ".github"), func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".yml") {
			files = append(files, path)
		}
		return err
	}))
	var found []string
	for _, file := range files {
		data, err := os.ReadFile(file)
		gotest.NoError(t, err)
		for i, line := range strings.Split(string(data), "\n") {
			if pinnedNpx.MatchString(line) {
				found = append(found, filepath.ToSlash(file)+":"+strconv.Itoa(i+1)+": "+strings.TrimSpace(line))
			}
		}
	}
	gotest.Empty(t, found, "npx tools carry a version; they run at their latest")
}

// A red cell without its event stream cannot be told from a slow one.
func (s *WorkflowsTestSuite) TestEveryGateCellKeepsItsEventStream(t *gotest.T) {
	wf := readWorkflow(t, "test.yml")
	for _, name := range []string{"test", "platform-test"} {
		steps := wf.Jobs[name].Steps
		at := slices.IndexFunc(steps, func(st workflowStep) bool { return st.Uses == "./" })
		gotest.GreaterOrEqual(t, at, 0, "%s has no step that runs the action", name)
		gotest.Equal(t, "test", steps[at].ID, "%s", name)
		gotest.NotEmpty(t, steps[at].With["events-file"], "%s saves no event stream", name)
		gotest.Less(t, at+1, len(steps), "%s", name)
		upload := steps[at+1]
		gotest.Regexp(t, `^actions/upload-artifact@`, upload.Uses, "%s", name)
		gotest.Contains(t, upload.If, "always()", "%s", name)
		gotest.Equal(t, "${{ steps.test.outputs.events-file }}", upload.With["path"], "%s", name)
	}
}

func (s *WorkflowsTestSuite) TestSlowRunnersGetScaledBudgets(t *gotest.T) {
	data, err := os.ReadFile(filepath.Join(workflowsDir, "test.yml"))
	gotest.NoError(t, err)
	gotest.NotContains(t, string(data), "timeout-scale: 1\n", "a hosted Windows or macOS runner is slower than the machine the budgets were written on")
}

func (s *WorkflowsTestSuite) TestPullRequestsJudgeTheMutantsTheirChangeTouches(t *gotest.T) {
	drill := readWorkflow(t, "quality.yml").Jobs["drill"]
	at := slices.IndexFunc(drill.Steps, func(st workflowStep) bool { return strings.Contains(st.Run, "DRILL_CHANGED=") })
	gotest.GreaterOrEqual(t, at, 0, "quality.yml's drill lists no changed files")
	gotest.Contains(t, drill.Steps[at].Run, "--no-renames")
	for _, name := range []string{"release.yml", "nightly.yml"} {
		data, err := os.ReadFile(filepath.Join(workflowsDir, name))
		gotest.NoError(t, err)
		gotest.NotContains(t, string(data), "DRILL_CHANGED", "%s must judge every mutant", name)
	}
}

func (s *WorkflowsTestSuite) TestTheContractRunsOnWhatTheExtensionReads(t *gotest.T) {
	wf := readWorkflow(t, "contract.yml")
	pr, push := eventPaths(t, wf, "pull_request"), eventPaths(t, wf, "push")
	gotest.Equal(t, pr, push, "contract.yml's push paths differ from its pull_request paths")
	gotest.Subset(t, pr, []string{"examples/**", "skills/**"})
	preflight, err := os.ReadFile(filepath.Join("..", "..", ".github", "scripts", "release-preflight.sh"))
	gotest.NoError(t, err)
	_, arm, found := strings.Cut(string(preflight), "contract.yml)")
	gotest.True(t, found, "release-preflight.sh has no contract.yml case")
	arm, _, _ = strings.Cut(arm, ";;\n\tesac")
	for _, p := range pr {
		gotest.Contains(t, arm, strings.TrimSuffix(p, "*"), "release-preflight.sh's gate_path does not cover %s", p)
	}
}
