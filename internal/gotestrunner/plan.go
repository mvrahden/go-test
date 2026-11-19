package gotestrunner

import (
	"maps"
	"slices"

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
				SharedKeys: sortedCopy(r.SuiteRequiredSharedFixtureKeys["Test"+name]),
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

// planFromMaps rebuilds a plan from the per-package maps.
func planFromMaps(o *OverlayResult) *RunPlan {
	harness := map[string]bool{}
	for _, pkg := range o.SuitePackages {
		harness[pkg] = true
	}
	paths := slices.Concat(o.SuitePackages, o.NoSuitePackages)
	if len(paths) == 0 {
		set := map[string]bool{}
		addKeys(set, o.SuitesByPkg)
		addKeys(set, o.BenchesByPkg)
		addKeys(set, o.DirsByPkg)
		addKeys(set, o.StdlibTestsByPkg)
		addKeys(set, o.SkippedSuitesByPkg)
		addKeys(set, o.FuzzFuncsByPkg)
		addKeys(set, o.GroupsByPkg)
		addKeys(set, o.ExclusiveSuitesByPkg)
		addKeys(set, o.TestlessSuitesByPkg)
		addKeys(set, o.SuiteRequiredSharedFixtureKeys)
		paths = slices.Sorted(maps.Keys(set))
	}
	p := &RunPlan{Packages: make([]PlannedPackage, 0, len(paths))}
	for _, path := range paths {
		pkg := PlannedPackage{
			Path:        path,
			Dir:         o.DirsByPkg[path],
			Harness:     harness[path],
			StdlibTests: o.StdlibTestsByPkg[path],
			Skipped:     o.SkippedSuitesByPkg[path],
		}
		benches := o.BenchesByPkg[path]
		suite := func(name string, tests bool) PlannedSuite {
			s := PlannedSuite{
				Package:    path,
				Dir:        o.DirsByPkg[path],
				Name:       name,
				Tests:      tests,
				Bench:      slices.Contains(benches, name),
				SharedKeys: sortedCopy(o.SuiteRequiredSharedFixtureKeys[path]["Test"+name]),
			}
			if tests {
				s.Testless = o.TestlessSuitesByPkg[path][name]
				s.Exclusive = o.ExclusiveSuitesByPkg[path][name]
				s.Group = o.GroupsByPkg[path][name]
			}
			for _, fn := range o.FuzzFuncsByPkg[path][name] {
				s.FuzzFuncs = append(s.FuzzFuncs, PlannedFuzz{Func: fn, Params: o.FuzzParamsByFunc[path][fn], Decoded: o.FuzzDecodedByFunc[path][fn]})
			}
			return s
		}
		suites := o.SuitesByPkg[path]
		for _, name := range suites {
			pkg.Suites = append(pkg.Suites, suite(name, true))
		}
		for _, name := range benches {
			if !slices.Contains(suites, name) {
				pkg.Suites = append(pkg.Suites, suite(name, false))
			}
		}
		p.Packages = append(p.Packages, pkg)
	}
	p.index()
	return p
}

func addKeys[V any](set map[string]bool, m map[string]V) {
	for k := range m {
		set[k] = true
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

// HasBenchmarks reports whether any suite has an effective benchmark.
func (p *RunPlan) HasBenchmarks() bool {
	return slices.ContainsFunc(p.All(), func(s *PlannedSuite) bool { return s.Bench })
}
