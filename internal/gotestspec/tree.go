package gotestspec

import (
	"sort"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
)

type Status int

const (
	StatusNone Status = iota
	StatusPass
	StatusFail
	StatusSkip
)

type NodeKind int

const (
	KindUnknown NodeKind = iota
	KindFixture
	KindSuite
	KindMethod
	KindBlock
	KindTest
)

// Declaration is what the source says about a subtest. Today that is the
// description as the developer wrote it; a run cannot report it, because by the
// time a subtest has a name the description has already been rewritten.
type Declaration struct {
	Label string
}

// DeclarationIndex maps a package's test paths ("TestUserServiceTestSuite/
// TestCreate/email_is_valid") to what source declares about them. The paths are
// the ones go test prints, which is what lets a declaration and an observation
// of the same behavior be one node rather than two.
type DeclarationIndex map[string]map[string]Declaration

func (i DeclarationIndex) lookup(pkgPath, testPath string) Declaration {
	if i == nil {
		return Declaration{}
	}
	return i[pkgPath][testPath]
}

type buildConfig struct {
	decls DeclarationIndex
}

type BuildOption func(*buildConfig)

// WithDeclarations supplies what the source declares, so the tree can show the
// developer's own words. Without it the tree falls back to reconstructing a
// label from the subtest name, which is lossy.
func WithDeclarations(idx DeclarationIndex) BuildOption {
	return func(c *buildConfig) { c.decls = idx }
}

type Node struct {
	Name    string
	Display string
	Kind    NodeKind
	// SourceLabel is the description as written, when it is known from source.
	// Rendering prefers it over reconstructing a label from Name: go test turns
	// every space in a description into an underscore, so the name cannot say
	// which underscores the developer typed. Empty for anything only a run
	// revealed.
	SourceLabel string
	Status      Status
	// Duration is what go test measured for this node alone. Start and End
	// bracket it in the stream. Each is wrong in its own way and EffectiveDuration
	// picks between them; see Paused.
	Duration time.Duration
	Start    time.Time
	End      time.Time
	// Paused records that the node called t.Parallel, so it was parked before it
	// ran. That makes its own bracket meaningless twice over: Start is when it
	// was registered rather than when it began, and go test flushes a parked
	// test's report through its parent, which can delay End by however long a
	// slower sibling runs — measured at 55s on a test that executed for 300ms.
	// Its Duration is exact, though, because go test stops counting while parked.
	//
	// The inverse holds for a node that was never parked: its Duration stops
	// when its function returns, which excludes any parallel children that run
	// afterwards — a suite of parallel methods measures ~0 — while its bracket
	// is clean, because go test reports a parent only once every descendant is
	// done. Descendants of a parked node keep clean timestamps; only the parked
	// node's own terminal event is delayed.
	Paused   bool
	Output   []string
	Children []*Node
	Focused  bool
	Excluded bool
	External bool
	// Incomplete marks a node whose children are known not to be the whole
	// list — a statically read method that declares behaviors whose names or
	// existence depend on runtime values. A tree stream never sets it, because
	// what ran is by definition all there was.
	Incomplete bool
	Variant    int
	duplicate  bool
}

type Package struct {
	Path     string
	Status   Status
	Duration time.Duration
	Nodes    []*Node
	Output   []string
}

type Stats struct {
	Suites    int
	Behaviors int
	Tests     int
	Passed    int
	Failed    int
	Skipped   int
	// FailedPackages counts packages whose verdict sits on the package itself
	// — a build failure, a TestMain os.Exit, a crash outside any test. These
	// carry no failing behavior, so folding them into Failed would break the
	// Passed+Failed+Skipped arithmetic; they are counted as what they are.
	FailedPackages int
}

func (s Stats) Total() int {
	return s.Passed + s.Failed + s.Skipped
}

func BuildTree(events []TestEvent, opts ...BuildOption) []*Package {
	var cfg buildConfig
	for _, o := range opts {
		o(&cfg)
	}

	pkgs := map[string]*Package{}
	nodes := map[string]map[string]*Node{}
	// Track top-level test run counts per package to detect ptest/pxtest duplicates.
	topRunCount := map[string]map[string]int{}

	for _, ev := range events {
		pkg := pkgs[ev.Package]
		if pkg == nil {
			pkg = &Package{Path: ev.Package}
			pkgs[ev.Package] = pkg
			nodes[ev.Package] = map[string]*Node{}
			topRunCount[ev.Package] = map[string]int{}
		}

		if ev.Test == "" {
			switch ev.Action {
			case ActionPass, ActionFail:
				pkg.Status = statusFrom(ev.Action)
				pkg.Duration = elapsed(ev.Elapsed)
			case ActionOutput:
				if !protocol.IsPackageSummaryLine(ev.Output) {
					pkg.Output = append(pkg.Output, ev.Output)
				}
			}
			continue
		}

		segments := splitTestPath(ev.Test)
		nmap := nodes[ev.Package]

		// Detect duplicate top-level run (ptest/pxtest same-name suite).
		if ev.Action == ActionRun && len(segments) == 1 {
			topRunCount[ev.Package][segments[0]]++
			if topRunCount[ev.Package][segments[0]] > 1 {
				// Create a duplicate node; children with #NN suffixes will attach here.
				dup := &Node{Name: segments[0], duplicate: true, Start: ev.Time}
				dupPath := segments[0] + "\x00dup"
				nmap[dupPath] = dup
				pkg.Nodes = append(pkg.Nodes, dup)
				continue
			}
		}

		// Resolve parent for children with #NN suffix (belongs to duplicate node).
		resolvedSegments := segments
		if len(segments) > 1 {
			resolvedSegments = resolveDuplicateSegments(segments, nmap)
		}

		// The index is keyed by the path go test would print for a first,
		// undisambiguated run, so duplicate bookkeeping is stripped before
		// lookup: a suite that runs twice declares its behaviors once.
		cleanSegments := make([]string, len(resolvedSegments))
		for i, seg := range resolvedSegments {
			cleanSegments[i] = stripDuplicateSuffix(strings.TrimSuffix(seg, "\x00dup"))
		}

		for i := range resolvedSegments {
			path := strings.Join(resolvedSegments[:i+1], "/")
			if nmap[path] != nil {
				continue
			}
			name := resolvedSegments[i]
			// Strip #NN suffix from display for children of duplicate runs.
			cleanName := stripDuplicateSuffix(name)
			n := &Node{
				Name:        cleanName,
				SourceLabel: cfg.decls.lookup(ev.Package, strings.Join(cleanSegments[:i+1], "/")).Label,
			}
			nmap[path] = n
			if i == 0 {
				pkg.Nodes = append(pkg.Nodes, n)
			} else {
				parent := nmap[strings.Join(resolvedSegments[:i], "/")]
				parent.Children = append(parent.Children, n)
			}
		}

		node := nmap[strings.Join(resolvedSegments, "/")]
		// Route top-level pass/fail/output to duplicate if original already resolved.
		if len(resolvedSegments) == 1 && node.Status != StatusNone {
			dupPath := resolvedSegments[0] + "\x00dup"
			if dup := nmap[dupPath]; dup != nil {
				node = dup
			}
		}
		switch ev.Action {
		case ActionOutput:
			node.Output = append(node.Output, ev.Output)
		case ActionRun:
			if node.Start.IsZero() {
				node.Start = ev.Time
			}
		case ActionPause, ActionCont:
			node.Paused = true
		case ActionPass, ActionFail, ActionSkip:
			node.Status = statusFrom(ev.Action)
			node.Duration = elapsed(ev.Elapsed)
			node.End = ev.Time
		}
	}

	for _, pkg := range pkgs {
		seen := map[string]int{}
		for _, n := range pkg.Nodes {
			classify(n, true)
			seen[n.Name]++
			if n.duplicate {
				n.Variant = seen[n.Name]
				n.External = true
			}
		}
	}

	result := make([]*Package, 0, len(pkgs))
	for _, pkg := range pkgs {
		// A package with no test nodes is usually noise ("no test files") — but
		// only when it carries no verdict. A node-less FAIL is a package that
		// died outside any test (a TestMain os.Exit(1), a run-level failure
		// appended after the stream); dropping it hid the failure from every
		// renderer while the exit code stayed red.
		if len(pkg.Nodes) == 0 && pkg.Status != StatusFail {
			continue
		}
		result = append(result, pkg)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Path < result[j].Path
	})
	return result
}

// resolveDuplicateSegments checks if a child path belongs to a duplicate
// top-level node (parent has #NN suffix pattern) and remaps it.
func resolveDuplicateSegments(segments []string, nmap map[string]*Node) []string {
	topName := segments[0]
	dupPath := topName + "\x00dup"
	if nmap[dupPath] == nil {
		return segments
	}

	// Check if any child segment has the #NN suffix indicating it belongs to
	// the duplicate run.
	for _, seg := range segments[1:] {
		if hasDuplicateSuffix(seg) {
			out := make([]string, len(segments))
			out[0] = topName + "\x00dup"
			for i := 1; i < len(segments); i++ {
				out[i] = stripDuplicateSuffix(segments[i])
			}
			return out
		}
	}
	return segments
}

func hasDuplicateSuffix(s string) bool {
	idx := strings.LastIndex(s, "#")
	if idx < 0 {
		return false
	}
	suffix := s[idx+1:]
	if len(suffix) == 0 {
		return false
	}
	for _, c := range suffix {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// StripDuplicateSuffix removes the "#01" the testing package appends to a
// subtest whose name repeats among its siblings. The tree shows both under the
// name the developer wrote, so a tree assembled from a source other than a test
// stream has to drop the suffix the same way rather than inventing its own rule.
func StripDuplicateSuffix(s string) string { return stripDuplicateSuffix(s) }

func stripDuplicateSuffix(s string) string {
	idx := strings.LastIndex(s, "#")
	if idx <= 0 {
		return s
	}
	suffix := s[idx+1:]
	for _, c := range suffix {
		if c < '0' || c > '9' {
			return s
		}
	}
	return s[:idx]
}

func CollectStats(packages []*Package) Stats {
	var s Stats
	for _, pkg := range packages {
		if PkgFailedOnItsOwn(pkg) {
			s.FailedPackages++
		}
		for _, n := range pkg.Nodes {
			collectStats(n, &s, n.Kind == KindTest)
		}
	}
	return s
}

// PkgFailedOnItsOwn reports whether a package's FAIL verdict originates on the
// package itself rather than on any test beneath it — the package-level
// counterpart of failedOnItsOwn. This is how a build failure, a TestMain
// os.Exit, or a crash outside any test presents: package status FAIL with no
// failing node to account for it.
func PkgFailedOnItsOwn(pkg *Package) bool {
	if pkg.Status != StatusFail {
		return false
	}
	for _, n := range pkg.Nodes {
		if n.Status == StatusFail || anyDescendantFailed(n) {
			return false
		}
	}
	return true
}

func collectStats(n *Node, s *Stats, inStdlib bool) {
	if n.Kind == KindSuite {
		s.Suites++
	}
	if failedOnItsOwn(n) {
		// Counted alongside the leaves so that Passed+Failed+Skipped still adds
		// up to the number reported: the verdict is on this node, and the tree
		// marks it here rather than under any of its children.
		if inStdlib {
			s.Tests++
		} else {
			s.Behaviors++
		}
		s.Failed++
	}
	if len(n.Children) == 0 {
		if inStdlib {
			s.Tests++
		} else {
			s.Behaviors++
		}
		switch n.Status {
		case StatusPass:
			s.Passed++
		case StatusFail:
			s.Failed++
		case StatusSkip:
			s.Skipped++
		}
	}
	for _, c := range n.Children {
		collectStats(c, s, inStdlib)
	}
}

func classify(n *Node, topLevel bool) {
	name := n.Name

	if topLevel {
		raw := strings.TrimPrefix(name, "Test")

		if strings.HasPrefix(raw, protocol.PrefixFocused) {
			n.Focused = true
			raw = strings.TrimPrefix(raw, protocol.PrefixFocused)
		} else if strings.HasPrefix(raw, protocol.PrefixExcluded) {
			n.Excluded = true
			raw = strings.TrimPrefix(raw, protocol.PrefixExcluded)
		}

		switch {
		case strings.HasPrefix(raw, "_") && strings.HasSuffix(raw, protocol.SuffixFixture):
			n.Kind = KindFixture
			n.Display = strings.TrimSuffix(strings.TrimPrefix(raw, "_"), protocol.SuffixFixture)
		case strings.HasSuffix(raw, protocol.SuffixTestSuite):
			n.Kind = KindSuite
			n.Display = strings.TrimSuffix(raw, protocol.SuffixTestSuite)
		default:
			n.Kind = KindTest
			n.Display = strings.TrimPrefix(raw, "_")
		}
	} else {
		if strings.HasPrefix(name, protocol.PrefixFocused) {
			n.Focused = true
			name = strings.TrimPrefix(name, protocol.PrefixFocused)
		} else if strings.HasPrefix(name, protocol.PrefixExcluded) {
			n.Excluded = true
			name = strings.TrimPrefix(name, protocol.PrefixExcluded)
		}

		switch {
		case strings.HasPrefix(name, "Test"):
			n.Kind = KindMethod
			n.Display = strings.TrimPrefix(name, "Test")
		case strings.HasSuffix(name, protocol.SuffixFixture) && !strings.HasSuffix(name, protocol.SuffixTestSuite):
			n.Kind = KindFixture
			n.Display = strings.TrimSuffix(name, protocol.SuffixFixture)
		case strings.HasSuffix(name, protocol.SuffixTestSuite):
			n.Kind = KindSuite
			n.Display = strings.TrimSuffix(name, protocol.SuffixTestSuite)
		default:
			n.Kind = KindBlock
			// Prefer the description as written. Reconstructing it from the
			// name turns every underscore into a space, which is right for the
			// ones go test put there and wrong for the ones the developer did.
			if n.SourceLabel != "" {
				n.Display = n.SourceLabel
			} else {
				n.Display = strings.ReplaceAll(name, "_", " ")
			}
		}
	}

	if n.Display == "" {
		n.Display = n.Name
	}

	for _, c := range n.Children {
		classify(c, false)
	}
}

// splitTestPath splits a go test -json Test field into subtest segments.
// Go uses "/" as the subtest separator, but description strings may contain
// "/" too (e.g. "https:// URI"). We treat consecutive slashes as literal
// characters within a segment rather than multiple separators.
func splitTestPath(path string) []string { return protocol.SplitTestPath(path) }

// failedOnItsOwn reports whether an interior node carries a verdict of its own:
// a suite whose AfterAll failed, a test method that blew its configured
// Timeout, or a bare t.Fail() with no message at all.
//
// A failed child marks its whole ancestry FAIL too, so the discriminator is
// that nothing beneath this node failed — then the FAIL can only have
// originated here, and the verdict comes from status alone. An earlier shape
// additionally required surviving output, which made a message-less failure
// vanish from the count and render an all-green summary beside a red exit
// code; prose is for the renderer, not the verdict. It still means a teardown
// failure that coincides with a test failure is not counted separately; the
// leaf already fails the run, and the node's own output is still rendered.
func failedOnItsOwn(n *Node) bool {
	return len(n.Children) > 0 &&
		n.Status == StatusFail &&
		!anyDescendantFailed(n)
}

// noDiagnosticNote is rendered in place of output for a failure that produced
// none — a bare Fail/FailNow — so a counted failure is never invisible.
const noDiagnosticNote = "test failed without diagnostic output (bare Fail?)"

// HasFailures reports whether the tree carries any failure: a failed behaviour,
// or a package that failed with every test passing (a TestMain os.Exit(1), a
// panic outside any test). It is the single exit rule for commands fed a
// recorded stream, so `spec --input` and `summary --input` cannot disagree
// about what CI sees.
func HasFailures(packages []*Package) bool {
	if CollectStats(packages).Failed > 0 {
		return true
	}
	for _, pkg := range packages {
		if pkg.Status == StatusFail {
			return true
		}
	}
	return false
}

func anyDescendantFailed(n *Node) bool {
	for _, child := range n.Children {
		if child.Status == StatusFail || anyDescendantFailed(child) {
			return true
		}
	}
	return false
}

// hasOwnDiagnostic reports whether an interior node carries output of its own,
// whatever else failed beneath it. It is the rendering condition: a suite whose
// AfterAll failed must show that error even when a behaviour under it failed
// too, because nothing else in the run will say the teardown broke.
//
// Counting uses the stricter failedOnItsOwn instead — a spurious count breaks
// the arithmetic the summary reports, while a spurious line of output only
// adds noise.
func hasOwnDiagnostic(n *Node) bool {
	return len(n.Children) > 0 && n.Status == StatusFail && len(filterOutput(n.Output)) > 0
}

func statusFrom(a Action) Status {
	switch a {
	case ActionPass:
		return StatusPass
	case ActionFail:
		return StatusFail
	case ActionSkip:
		return StatusSkip
	}
	return StatusNone
}

func elapsed(s float64) time.Duration {
	return time.Duration(s * float64(time.Second))
}

// EffectiveDuration is the wall clock a node occupied: an interval, never a sum
// of its children, which is an integral over occupancy and exceeds the clock as
// soon as anything runs in parallel.
//
// Which of the node's two measures to believe follows from why each one lies.
// A parked node reports what go test measured, because its bracket carries the
// wait for a slot and go test's delayed flush. Everything else reports its
// bracket, because a measure that stops when the function returns cannot see
// parallel children, while the bracket encloses the whole subtree — and covers
// work the node did outside any child, such as the subprocess a When starts
// before its assertions.
func EffectiveDuration(n *Node) time.Duration {
	if n.Paused {
		return max(n.Duration, childFloor(n))
	}
	if d, ok := bracket(n); ok {
		return max(d, childFloor(n))
	}
	if len(n.Children) == 0 {
		return n.Duration
	}
	// A tree assembled from something other than a stream — a static spec, a
	// hand-built fixture — carries no timestamps. The subtree is then bounded
	// below by both the node's own measure and its children's.
	sum := time.Duration(0)
	for _, c := range n.Children {
		sum += EffectiveDuration(c)
	}
	return max(n.Duration, sum)
}

// childFloor is the least a node can have cost. A child runs inside its parent,
// so the parent is at least as long as any single one of them, and at least as
// long as the clock its placeable children occupied between them. Flooring by
// that repairs the rounding go test applies to a parked node's measure — it
// reports to 10ms, while a child's bracket is exact — without ever inventing
// time, because neither bound can exceed what the parent actually ran for.
func childFloor(n *Node) time.Duration {
	var floor time.Duration
	for _, c := range n.Children {
		floor = max(floor, EffectiveDuration(c))
	}
	if d, ok := unionOf(n.Children); ok {
		floor = max(floor, d)
	}
	return floor
}

func bracket(n *Node) (time.Duration, bool) {
	if n.Start.IsZero() || n.End.IsZero() || n.End.Before(n.Start) {
		return 0, false
	}
	return n.End.Sub(n.Start), true
}

// interval is the window a node occupied, for rows that have to be assembled
// out of several nodes. A parked node cannot be placed on the clock at all —
// its Duration says how long it ran but both its endpoints are unreliable — so
// it declines rather than contributing a window in the wrong place. Nothing is
// lost by that: only top-level nodes are ever unioned, and a suite is not
// parked, so the parked methods inside it sit within a window that is.
func interval(n *Node) (start, end time.Time, ok bool) {
	if n.Paused || n.Start.IsZero() || n.End.IsZero() || n.End.Before(n.Start) {
		return
	}
	return n.Start, n.End, true
}

// PackageDuration is how long a package was busy. A package is not something
// that executes, so it has no bracket of its own — the runner dispatches its
// suites among every other package's, and the summary event it emits carries
// the sum of their process times rather than any interval. What is left, and
// what is true, is the union of the windows its suites occupied.
func PackageDuration(pkg *Package) time.Duration {
	if d, ok := unionOf(pkg.Nodes); ok {
		return d
	}
	return pkg.Duration
}

// TotalDuration is the union across every package: the wall clock during which
// this run was executing something. Gaps are excluded, which is what makes the
// number survive a tree holding results from more than one run.
func TotalDuration(packages []*Package) time.Duration {
	all := make([]*Node, 0, len(packages))
	for _, pkg := range packages {
		all = append(all, pkg.Nodes...)
	}
	if d, ok := unionOf(all); ok {
		return d
	}
	var sum time.Duration
	for _, pkg := range packages {
		sum += pkg.Duration
	}
	return sum
}

// unionOf merges the nodes' windows and totals what is left. Overlap is counted
// once: two suites that ran side by side for a second cost a second, not two.
func unionOf(nodes []*Node) (time.Duration, bool) {
	type window struct{ start, end time.Time }
	windows := make([]window, 0, len(nodes))
	for _, n := range nodes {
		if s, e, ok := interval(n); ok {
			windows = append(windows, window{s, e})
		}
	}
	if len(windows) == 0 {
		return 0, false
	}
	sort.Slice(windows, func(i, j int) bool { return windows[i].start.Before(windows[j].start) })

	var total time.Duration
	cur := windows[0]
	for _, w := range windows[1:] {
		if w.start.After(cur.end) {
			total += cur.end.Sub(cur.start)
			cur = w
			continue
		}
		if w.end.After(cur.end) {
			cur.end = w.end
		}
	}
	return total + cur.end.Sub(cur.start), true
}

// ClassifyRoots applies the tree's naming rules — kind, display text, focus and
// exclusion prefixes — to nodes assembled from a source other than a test
// stream. A statically derived tree must classify identically to an observed
// one, so both go through this single implementation rather than duplicating
// the rules.
func ClassifyRoots(nodes []*Node) {
	for _, n := range nodes {
		classify(n, true)
	}
}
