package gotestrunner

import (
	"maps"
	"slices"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestgen"
)

// RunPlan is every static fact a run reads about its packages and their
// suites, produced once from the generator's results.
type RunPlan struct {
	Packages []PlannedPackage // in the generator's order
	byPkg    map[string]*PlannedPackage
}

// PlannedPackage is one package that loaded.
type PlannedPackage struct {
	Path        string         // import path
	Dir         string         // source directory
	Harness     bool           // a harness was generated for it
	StdlibTests int            // stdlib tests gotest reports, never runs
	Skipped     []string       // suites focus or X_ left out
	Suites      []PlannedSuite // suites with a test run in order, then bench-only suites
}

// PlannedSuite is one suite a package declares.
type PlannedSuite struct {
	Package    string        // import path
	Dir        string        // the package's source directory
	Name       string        // suite type identifier, "FooTestSuite"
	Tests      bool          // has a test run: test methods or fuzz seeds
	Testless   bool          // its test run replays fuzz seeds only
	Bench      bool          // has an effective benchmark
	Exclusive  bool          // SuiteConfig{Exclusive: true}
	Group      string        // SuiteConfig.ExclusiveGroup, "" the default group
	FuzzFuncs  []PlannedFuzz // generated wrappers, in source order
	SharedKeys []string      // shared fixture state keys it reads, closed, sorted
}

// PlannedFuzz is one generated fuzz wrapper.
type PlannedFuzz struct {
	Func    string   // "Fuzz<Suite>_<Method>"
	Params  []string // corpus type of each value the engine feeds it
	Decoded bool     // its input is decoded from the engine's leaves
}

// TestFunc is the suite's generated test function.
func (s *PlannedSuite) TestFunc() string { return "Test" + s.Name }

// BenchFunc is the suite's generated benchmark function.
func (s *PlannedSuite) BenchFunc() string { return "Benchmark" + s.Name }

// FuzzNames lists the suite's fuzz wrappers by name.
func (s *PlannedSuite) FuzzNames() []string {
	var names []string
	for i := range s.FuzzFuncs {
		names = append(names, s.FuzzFuncs[i].Func)
	}
	return names
}

// planRun builds the plan. It is the only place a per-suite fact is read
// from the generator's lists.
func planRun(results gotestgen.GenerateResults) *RunPlan {
	p := &RunPlan{Packages: make([]PlannedPackage, 0, len(results))}
	for _, r := range results {
		pkg := PlannedPackage{
			Path:        r.PkgPath,
			Dir:         r.AbsPath,
			Harness:     len(r.PTest) > 0 || len(r.PXTest) > 0,
			StdlibTests: r.StdlibTestCount,
		}
		if len(r.SkippedSuiteNames) > 0 {
			pkg.Skipped = r.SkippedSuiteNames
		}
		suite := func(name string, tests bool) PlannedSuite {
			s := PlannedSuite{
				Package:    r.PkgPath,
				Dir:        r.AbsPath,
				Name:       name,
				Tests:      tests,
				Bench:      slices.Contains(r.BenchSuiteNames, name),
				SharedKeys: sortedCopy(r.SuiteRequiredSharedFixtureKeys[name]),
			}
			if tests {
				s.Testless = slices.Contains(r.TestlessSuiteNames, name)
				s.Exclusive = slices.Contains(r.ExclusiveSuiteNames, name)
				s.Group = r.GroupBySuite[name]
			}
			for _, fn := range r.FuzzFuncsBySuite[name] {
				s.FuzzFuncs = append(s.FuzzFuncs, PlannedFuzz{Func: fn, Params: r.FuzzParamsByFunc[fn], Decoded: r.FuzzDecodedFuncs[fn]})
			}
			return s
		}
		for _, name := range r.SuiteNames {
			pkg.Suites = append(pkg.Suites, suite(name, true))
		}
		for _, name := range r.BenchSuiteNames {
			if !slices.Contains(r.SuiteNames, name) {
				pkg.Suites = append(pkg.Suites, suite(name, false))
			}
		}
		p.Packages = append(p.Packages, pkg)
	}
	p.index()
	return p
}

func (p *RunPlan) index() {
	p.byPkg = make(map[string]*PlannedPackage, len(p.Packages))
	for i := range p.Packages {
		p.byPkg[p.Packages[i].Path] = &p.Packages[i]
	}
}

// Package returns the planned package at path; nil when the plan holds none.
func (p *RunPlan) Package(path string) *PlannedPackage {
	if p == nil {
		return nil
	}
	return p.byPkg[path]
}

// Suite returns the suite pkg declares by its identifier; nil when absent.
func (p *RunPlan) Suite(pkg, name string) *PlannedSuite {
	pp := p.Package(pkg)
	if pp == nil {
		return nil
	}
	for i := range pp.Suites {
		if pp.Suites[i].Name == name {
			return &pp.Suites[i]
		}
	}
	return nil
}

// All lists every suite, packages in plan order.
func (p *RunPlan) All() []*PlannedSuite {
	if p == nil {
		return nil
	}
	var out []*PlannedSuite
	for i := range p.Packages {
		for j := range p.Packages[i].Suites {
			out = append(out, &p.Packages[i].Suites[j])
		}
	}
	return out
}

// Fuzz returns the fuzz wrapper fn of pkg; nil when absent.
func (p *RunPlan) Fuzz(pkg, fn string) *PlannedFuzz {
	pp := p.Package(pkg)
	if pp == nil {
		return nil
	}
	for i := range pp.Suites {
		for j := range pp.Suites[i].FuzzFuncs {
			if pp.Suites[i].FuzzFuncs[j].Func == fn {
				return &pp.Suites[i].FuzzFuncs[j]
			}
		}
	}
	return nil
}

// StdlibTests counts, per package that has any, the stdlib tests gotest
// reports and never runs.
func (p *RunPlan) StdlibTests() map[string]int {
	out := map[string]int{}
	if p == nil {
		return out
	}
	for i := range p.Packages {
		if n := p.Packages[i].StdlibTests; n > 0 {
			out[p.Packages[i].Path] = n
		}
	}
	return out
}

// SkippedSuites lists, per package that has any, the suites focus or X_
// left out.
func (p *RunPlan) SkippedSuites() map[string][]string {
	out := map[string][]string{}
	if p == nil {
		return out
	}
	for i := range p.Packages {
		if len(p.Packages[i].Skipped) > 0 {
			out[p.Packages[i].Path] = p.Packages[i].Skipped
		}
	}
	return out
}

// HasBenchmarks reports whether any suite has an effective benchmark.
func (p *RunPlan) HasBenchmarks() bool {
	return slices.ContainsFunc(p.All(), func(s *PlannedSuite) bool { return s.Bench })
}

// HasFuzz reports whether any suite has a generated fuzz wrapper.
func (p *RunPlan) HasFuzz() bool {
	return slices.ContainsFunc(p.All(), func(s *PlannedSuite) bool { return len(s.FuzzFuncs) > 0 })
}

// selection is what a run's flags leave of the plan.
type selection struct {
	Run, Skip string // -run and -skip
	Bench     string // -bench, for a bench run
	Groups    groupSelection
}

// testRun lists the suites a test run dispatches: those with a test run,
// in a selected group, that -run enters and -skip does not drop whole.
func (p *RunPlan) testRun(sel selection) []*PlannedSuite { //nolint:gocritic // hugeParam: once per run
	var out []*PlannedSuite
	for _, s := range p.All() {
		if !s.Tests || !sel.Groups.selects(s.Group) {
			continue
		}
		funcs := suiteFuncs(s.Name, s.Testless, s.FuzzNames())
		if selectsSuite(sel.Run, funcs) && !skipsSuite(sel.Skip, funcs) {
			out = append(out, s)
		}
	}
	return out
}

// benchRun lists the suites a bench run dispatches, by (Package, Name).
func (p *RunPlan) benchRun(sel selection) []*PlannedSuite { //nolint:gocritic // hugeParam: once per run
	var out []*PlannedSuite
	for _, s := range p.All() {
		if s.Bench && selectsBench(sel.Run, sel.Bench, sel.Skip, s.BenchFunc()) {
			out = append(out, s)
		}
	}
	slices.SortFunc(out, func(a, b *PlannedSuite) int {
		if a.Package != b.Package {
			return strings.Compare(a.Package, b.Package)
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out
}

// Groups lists the named groups the plan's suites declare, sorted.
func (p *RunPlan) Groups() []string {
	set := map[string]bool{}
	for _, s := range p.All() {
		if s.Tests && s.Group != "" {
			set[s.Group] = true
		}
	}
	return slices.Sorted(maps.Keys(set))
}

// plannedKeys is the shared fixture keys the suite of t reads; a test
// target names its suite by its test function.
func plannedKeys(p *RunPlan, t *SuiteTarget) []string {
	if s := p.Suite(t.Package, strings.TrimPrefix(t.SuiteName, "Test")); s != nil {
		return s.SharedKeys
	}
	return nil
}

// byPackage groups suites by package, each package's in the given order.
func byPackage(suites []*PlannedSuite) map[string][]*PlannedSuite {
	out := map[string][]*PlannedSuite{}
	for _, s := range suites {
		out[s.Package] = append(out[s.Package], s)
	}
	return out
}

// flatten lists the suites of a package-keyed phase, packages sorted.
func flatten(phase map[string][]*PlannedSuite) []*PlannedSuite {
	var out []*PlannedSuite
	for _, pkg := range slices.Sorted(maps.Keys(phase)) {
		out = append(out, phase[pkg]...)
	}
	return out
}
