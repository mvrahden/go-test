package drill_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/mvrahden/go-test/internal/drill"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// PlanningTestSuite: the drill's planning and verdict accounting, which
// never apply or run a mutant.
type PlanningTestSuite struct{}

func (s *PlanningTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func shared(names ...string) map[string]string {
	modes := map[string]string{}
	for _, n := range names {
		modes[n] = "shared"
	}
	return modes
}

func (s *PlanningTestSuite) TestShardsCoverEveryMutantOnceAndBalanceLongJobs(t *gotest.T) {
	names := []string{"a", "b", "c", "d", "e"}
	weights := map[string]float64{"a": 100, "b": 80, "c": 20, "d": 10, "e": 10}
	result := drill.Schedule(names, shared(names...), weights, 2)
	gotest.Equal(t, names, sorted(slices.Concat(result...)))
	var loads []float64
	for _, shard := range result {
		load := 0.0
		for _, n := range shard {
			load += weights[n]
		}
		loads = append(loads, load)
	}
	gotest.Equal(t, []float64{110, 110}, loads)
	gotest.Equal(t, result, drill.Schedule(reversed(names), shared(names...), weights, 2))
}

func (s *PlanningTestSuite) TestShardsWithDifferentCPUCountsPlanAlike(t *gotest.T) {
	var names []string
	modes, weights := map[string]string{}, map[string]float64{}
	for i := range 40 {
		n := fmt.Sprintf("m%02d", i)
		names = append(names, n)
		modes[n] = "shared"
		if i%7 == 0 {
			modes[n] = "alone"
		}
		weights[n] = float64(10 + (i*37)%90)
	}
	entries := []struct {
		Desc    string
		environ map[string]string
		cpus    int
	}{
		{"two jobs on 8 CPUs", map[string]string{"DRILL_JOBS": "2"}, 8},
		{"four jobs on 8 CPUs", map[string]string{"DRILL_JOBS": "4"}, 8},
		{"default jobs on 4 CPUs", map[string]string{}, 4},
		{"default jobs on 12 CPUs", map[string]string{}, 12},
	}
	for t, entry := range gotest.Each(t, entries) {
		two, _, err := drill.Plan(names, modes, weights, 2, map[string]string{"DRILL_JOBS": "2"}, 4)
		gotest.NoError(t, err)
		other, _, err := drill.Plan(names, modes, weights, 2, entry.environ, entry.cpus)
		gotest.NoError(t, err)
		gotest.Equal(t, names, sorted(slices.Concat(two[0], other[1])))
		gotest.Equal(t, two, other)
	}
}

func (s *PlanningTestSuite) TestIsolatedTimeIsNotDividedByWorkerCount(t *gotest.T) {
	modes := map[string]string{"a": "alone", "b": "shared", "c": "shared"}
	weights := map[string]float64{"a": 10, "b": 10, "c": 10}
	gotest.Equal(t, [][]string{{"a"}, {"b", "c"}}, drill.Schedule([]string{"a", "b", "c"}, modes, weights, 2))
}

func selection(shard int, assignment [][]string, mutants []string) map[string]any {
	if mutants == nil {
		mutants = assignment[shard-1]
	}
	selected := map[string]any{}
	for _, m := range mutants {
		selected[m] = map[string]any{}
	}
	return map[string]any{"shard": shard, "shards": len(assignment), "assignment": assignment, "mutants": selected}
}

func (s *PlanningTestSuite) TestShardsThatDisagreeFailTheCheck(t *gotest.T) {
	agreed := [][]string{{"a", "c"}, {"b"}}
	entries := []struct {
		Desc       string
		selections []map[string]any
		problems   []string
	}{
		{"agree", []map[string]any{selection(1, agreed, nil), selection(2, agreed, nil)}, nil},
		{"different plans", []map[string]any{selection(1, agreed, nil), selection(2, [][]string{{"a"}, {"b", "c"}}, nil)},
			[]string{"shards computed different assignments"}},
		{"missing shard", []map[string]any{selection(1, agreed, nil)},
			[]string{"expected one selection per shard 1..2, got [1]"}},
		{"stray selection", []map[string]any{selection(1, agreed, []string{"a"}), selection(2, agreed, nil)},
			[]string{"shard 1 selected other mutants than its assignment names"}},
	}
	for t, entry := range gotest.Each(t, entries) {
		dir := t.TempDir()
		var paths []string
		for i, sel := range entry.selections {
			path := filepath.Join(dir, fmt.Sprintf("%d.json", i))
			data, err := json.Marshal(sel)
			gotest.NoError(t, err)
			gotest.NoError(t, os.WriteFile(path, data, 0o600))
			paths = append(paths, path)
		}
		gotest.Equal(t, entry.problems, drill.CheckShards(paths))
	}
}

func (s *PlanningTestSuite) TestIncompleteOrDuplicatePlansAreRejected(t *gotest.T) {
	entries := []struct{ Desc, text string }{
		{"no run", "mutant\ta\tshared\n"},
		{"duplicate mutant", "mutant\ta\tshared\nmutant\ta\tshared\n"},
		{"planner complaint", "bad\ta\tno such test\n"},
		{"run of another mutant", "mutant\ta\tshared\nrun\tb\t-\t./x\n"},
	}
	for t, entry := range gotest.Each(t, entries) {
		_, _, err := drill.ReadPlan(entry.text, []string{"a"})
		gotest.Error(t, err)
	}
}

func (s *PlanningTestSuite) TestAValidPlanPreservesLiteralFilters(t *gotest.T) {
	modes, runs, err := drill.ReadPlan("mutant\ta\talone\nrun\ta\t^(TestA)$/^(TestB)$\t./x ./y\n", []string{"a"})
	gotest.NoError(t, err)
	gotest.Equal(t, map[string]string{"a": "alone"}, modes)
	gotest.Equal(t, []drill.Run{{Pattern: "^(TestA)$/^(TestB)$", Packages: []string{"./x", "./y"}}}, runs["a"])
}

func (s *PlanningTestSuite) TestABuildFailureCannotBeAcceptedAsAnAssertionFailure(t *gotest.T) {
	broken, err := drill.BuildFailure(`{"Action":"build-fail","ImportPath":"p"}` + "\n")
	gotest.NoError(t, err)
	gotest.True(t, broken)
	broken, err = drill.BuildFailure(`{"Action":"fail","Test":"TestA"}` + "\n")
	gotest.NoError(t, err)
	gotest.False(t, broken)
	_, err = drill.BuildFailure("not JSON")
	gotest.Error(t, err)
}

func (s *PlanningTestSuite) TestTheJudgeRunsWithoutTheHostsCISettings(t *gotest.T) {
	host := map[string]string{"PATH": "/bin", "CI": "true", "GOTEST_CI": "1", "GOTEST_UPDATE_SNAPSHOTS": "1",
		"GOTEST_CACHE_DIR": "/shared", "GITHUB_ACTIONS": "true", "GOFLAGS": "-mod=mod"}
	env := drill.WorkerEnv(host, "/w", "/w/tmp", 3)
	for _, name := range []string{"CI", "GOTEST_CI", "GOTEST_UPDATE_SNAPSHOTS", "GITHUB_ACTIONS"} {
		gotest.NotContains(t, env, name)
	}
	gotest.Equal(t, filepath.Join("/w", "cache", "judge"), env["GOTEST_CACHE_DIR"])
	gotest.Equal(t, []string{"/bin", "-mod=mod -p=3", "3"}, []string{env["PATH"], env["GOFLAGS"], env["GOMAXPROCS"]})
}

func exit(code int) *int { return &code }

func (s *PlanningTestSuite) TestAMissingOrStaleResultAndADeadWorkerFailClosed(t *gotest.T) {
	path := filepath.Join(t.TempDir(), "result")
	clean := &drill.Status{Exit: exit(0)}
	gotest.False(t, drill.ReadResult(path, "a", clean, 0).Caught)
	gotest.NoError(t, os.WriteFile(path, []byte(`{"name": "b", "caught": true}`), 0o600))
	gotest.False(t, drill.ReadResult(path, "a", clean, 0).Caught)
	gotest.NoError(t, os.WriteFile(path, []byte(`{"name": "a", "caught": true}`), 0o600))
	gotest.True(t, drill.ReadResult(path, "a", clean, 0).Caught)
	entries := []struct {
		Desc   string
		status *drill.Status
		code   int
	}{
		{"no status", nil, 0},
		{"empty status", &drill.Status{}, 0},
		{"killed worker", &drill.Status{Exit: exit(-9)}, 0},
		{"timeout", &drill.Status{Exit: exit(0), Timeout: true}, 0},
		{"failed cleanup", &drill.Status{Exit: exit(0), CleanupError: "failed"}, 0},
		{"failed supervisor", clean, 1},
	}
	for t, entry := range gotest.Each(t, entries) {
		gotest.False(t, drill.ReadResult(path, "a", entry.status, entry.code).Caught)
	}
}

func sorted(s []string) []string {
	s = slices.Clone(s)
	slices.Sort(s)
	return s
}

func reversed(s []string) []string {
	s = slices.Clone(s)
	slices.Reverse(s)
	return s
}
