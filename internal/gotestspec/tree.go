package gotestspec

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

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
	// KindBenchmark is appended at the end of the iota block to keep the
	// numeric values of the existing kinds stable across serialized state.
	KindBenchmark
	// KindFuzz is a suite's fuzz target: a generated Fuzz<Suite>_<Method>
	// wrapper re-homed under its suite, its seed replays as children.
	KindFuzz
)

// Vocab records which gotest call declared a subtest. Like the description, it
// is a property of the source: `t.When(...)` is visible long before anything
// executes, and a run cannot report it because a context and an expectation
// both arrive as plain subtests.
type Vocab int

const (
	VocabNone Vocab = iota
	VocabWhen
	VocabIt
	VocabEach
)

// Apply renders a label in its vocabulary. A When block reads as "when email is
// valid"; It and Each labels stand on their own, because the ✓/✗ glyph already
// plays the role of "it". A label that already opens with a connective of its
// own — "when", but also "with", "given", "after", … — is left alone rather
// than doubled: "when with an empty cache" is not a sentence.
func (v Vocab) Apply(label string) string {
	if v != VocabWhen || opensWithConnective(label) {
		return label
	}
	return "when " + label
}

// connectives are the words that already turn a label into a clause, so no
// "when" goes in front of them. Matched as whole words, case-insensitively.
var connectives = []string{
	"when", "whenever", "with", "without", "given", "if", "unless",
	"after", "before", "while", "once", "on", "upon", "as", "for",
	"during", "under",
}

// opensWithConnective reports whether a label's first word is a connective.
// The separator may be a space or an underscore: the same label reaches this
// rule both as the developer typed it and as it survives a subtest name, where
// every space is an underscore. Judging the two differently is how one behavior
// ends up spelled two ways.
func opensWithConnective(label string) bool {
	word := label
	if i := strings.IndexAny(label, " _\t"); i >= 0 {
		word = label[:i]
	}
	return slices.ContainsFunc(connectives, func(c string) bool { return strings.EqualFold(word, c) })
}

// Declaration is what the source says about a subtest: the description as the
// developer wrote it, and which call declared it. A run can report neither —
// by the time a subtest has a name, the description has been rewritten and the
// call that made it is gone.
type Declaration struct {
	Label string
	Vocab Vocab
}

// DeclKey keys a declaration by its written levels from the top-level test
// down; "a/b" as one level and "a" then "b" are different keys.
func DeclKey(levels ...string) string { return strings.Join(levels, "\x00") }

// DeclarationIndex maps a package's declarations, by DeclKey of their levels
// ("TestUserServiceTestSuite", "TestCreate", "email_is_valid"), to what source
// declares about them. The levels are the ones go test prints, which is what
// lets a declaration and an observation of the same behavior be one node
// rather than two.
type DeclarationIndex map[string]map[string]Declaration

func (i DeclarationIndex) lookup(pkgPath, key string) Declaration {
	if i == nil {
		return Declaration{}
	}
	return i[pkgPath][key]
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
	// Vocab is the call that declared this node, when it is known from source.
	Vocab Vocab
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
	// NotRun marks a node the source declares that the stream laid over it
	// did not report (see Overlay). It has no verdict and no duration.
	NotRun bool
	// File and Line say where the source declares the node: File relative
	// to the package directory, as gotest discover gives it. Empty for a
	// node only a run revealed.
	File    string
	Line    int
	Variant int
	// Reported is the own level go test reported when it is not Name: a
	// twin's TestCreate#01, whose #01 is testing's, not the developer's.
	Reported    string
	Iterations  int
	NsPerOp     float64
	BytesPerOp  int64
	AllocsPerOp int64
	// Metrics are the result line's other "<value> <unit>" pairs: MB/s and
	// what ReportMetric added.
	Metrics map[string]float64
	// Procs is the GOMAXPROCS suffix of the result line.
	Procs int
}

type Package struct {
	Path string
	// Dir is the package's directory, when the tree was read from source.
	Dir string
	// RunLevel marks a package the run booked for itself (shared fixtures,
	// an interrupt, …): its events said so.
	RunLevel bool
	Status   Status
	Duration time.Duration
	Nodes    []*Node
	Output   []string
}

type Stats struct {
	Suites     int
	Behaviors  int
	Tests      int
	Benchmarks int
	Fuzzers    int
	Passed     int
	Failed     int
	Skipped    int
	// FailedPackages counts packages whose verdict sits on the package itself
	// — a build failure, a TestMain os.Exit, a crash outside any test. These
	// carry no failing behavior, so folding them into Failed would break the
	// Passed+Failed+Skipped arithmetic; they are counted as what they are.
	FailedPackages int
	// NotRun counts the behaviors and fuzz targets without a verdict: declared
	// and not reported, or started and never ended.
	NotRun int
}

func (s Stats) Total() int { //nolint:gocritic // hugeParam: stable API
	return s.Passed + s.Failed + s.Skipped
}

func BuildTree(events []TestEvent, opts ...BuildOption) []*Package {
	var cfg buildConfig
	for _, o := range opts {
		o(&cfg)
	}

	pkgs := map[string]*Package{}
	builds := map[string]*treeBuild{}
	// Build diagnostics are keyed by build, not by package: they go to every
	// package that build failed.
	buildOutput := map[string][]string{}
	failedBuild := map[string]string{}
	benchScans := map[*Node]*benchScan{}
	stamped := stampedPackages(events)

	for i := range events {
		ev := &events[i]
		if ev.Action == ActionBuildOutput || ev.Action == ActionBuildFail {
			if ev.Output != "" {
				buildOutput[ev.ImportPath] = append(buildOutput[ev.ImportPath], ev.Output)
			}
			continue
		}
		pkg := pkgs[ev.Package]
		if pkg == nil {
			pkg = &Package{Path: ev.Package}
			pkgs[ev.Package] = pkg
			builds[ev.Package] = newTreeBuild(pkg, stamped[ev.Package])
		}
		if ev.RunLevel {
			pkg.RunLevel = true
		}

		if ev.Test == "" {
			switch ev.Action {
			case ActionPass, ActionFail:
				pkg.Status = statusFrom(ev.Action)
				pkg.Duration = elapsed(ev.Elapsed)
				if ev.FailedBuild != "" {
					failedBuild[ev.Package] = ev.FailedBuild
				}
			case ActionOutput:
				if !protocol.IsPackageSummaryLine(ev.Output) {
					pkg.Output = append(pkg.Output, ev.Output)
				}
			}
			continue
		}

		e := builds[ev.Package].place(ev)
		node := e.node
		switch ev.Action {
		case ActionOutput:
			node.Output = append(node.Output, ev.Output)
			// A sub-benchmark's level is whatever b.Run was given; the
			// benchmark above it is what makes its output a result line.
			if e.underBenchmark() {
				scan := benchScans[node]
				if scan == nil {
					scan = &benchScan{}
					benchScans[node] = scan
				}
				scan.read(node, ev.Output)
			}
		case ActionRun:
			if node.Start.IsZero() {
				node.Start = ev.Time
			}
		case ActionPause, ActionCont:
			node.Paused = true
		case ActionBench:
			if node.Status == StatusNone {
				node.Status = StatusPass
			}
		case ActionPass, ActionFail, ActionSkip:
			node.Status = statusFrom(ev.Action)
			node.Duration = elapsed(ev.Elapsed)
			node.End = ev.Time
		}
	}

	for node, scan := range benchScans {
		scan.end(node)
	}
	for path, build := range failedBuild {
		pkgs[path].Output = append(slices.Clone(buildOutput[build]), pkgs[path].Output...)
	}

	for path, pkg := range pkgs {
		builds[path].prune()
		if cfg.decls != nil {
			declare(pkg.Nodes, nil, func(levels []string) Declaration {
				return cfg.decls.lookup(path, DeclKey(levels...))
			})
		}
		attachFuzzWrappers(pkg)
		// Completion order is dispatch order, which parallel suite processes
		// shuffle; by name, two runs of one package render alike. A suite run
		// twice (ptest and pxtest) keeps its first run first.
		sort.SliceStable(pkg.Nodes, func(i, j int) bool {
			a, b := pkg.Nodes[i], pkg.Nodes[j]
			if a.Name != b.Name {
				return a.Name < b.Name
			}
			return a.Variant < b.Variant
		})
		for _, n := range pkg.Nodes {
			classify(n, true)
		}
		FailBranches(pkg)
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

// stampedPackages are the packages one of whose runs carries a Subtest:
// their events say where each test sits. An unstamped run among them falls
// back to the split, as a stream composed from an older CLI's has.
func stampedPackages(events []TestEvent) map[string]bool {
	stamped := map[string]bool{}
	for i := range events {
		ev := &events[i]
		if ev.Action == ActionRun && ev.Gotest != nil && ev.Gotest.Subtest != "" {
			stamped[ev.Package] = true
		}
	}
	return stamped
}

// treeBuild assembles one package's nodes. A node is keyed by its instance
// and full name, which testing keeps unique within an instance; where it
// sits comes from the stamps, else from the Placer, which reads the
// stream's order.
type treeBuild struct {
	pkg     *Package
	placer  *protocol.Placer // nil when the package is stamped
	placed  map[treeKey]protocol.Placement
	entries map[treeKey]*treeEntry
}

type treeKey struct {
	instance int
	test     string
}

type treeEntry struct {
	node   *Node
	parent *treeEntry // nil for a top-level node
	// placeholder marks a node made before its own run: for a child whose
	// parent the stream had not started, or an event without a run.
	placeholder bool
	reported    bool // an event of its own arrived
}

func newTreeBuild(pkg *Package, stamped bool) *treeBuild {
	b := &treeBuild{pkg: pkg, entries: map[treeKey]*treeEntry{}}
	if stamped {
		b.placed = map[treeKey]protocol.Placement{}
	} else {
		b.placer = protocol.NewPlacer()
	}
	return b
}

// placement is where ev's test sits. A stamped non-run event takes its
// run's placement; one that comes before its run is guessed, and the run
// replaces the guess.
func (b *treeBuild) placement(ev *TestEvent) protocol.Placement {
	if b.placer != nil {
		return b.placer.Place(string(ev.Action), ev.Test)
	}
	var st protocol.Stamp
	if ev.Gotest != nil {
		st = *ev.Gotest
	}
	if ev.Action == ActionRun {
		pl := protocol.PlacementFromStamp(ev.Test, st)
		b.placed[treeKey{pl.Instance, ev.Test}] = pl
		return pl
	}
	n := max(st.Instance, 1)
	if pl, ok := b.placed[treeKey{n, ev.Test}]; ok {
		return pl
	}
	return protocol.Guess(ev.Test, n)
}

// place reads one test event and returns its node.
func (b *treeBuild) place(ev *TestEvent) *treeEntry {
	action, test := ev.Action, ev.Test
	pl := b.placement(ev)
	key := treeKey{pl.Instance, test}
	e := b.entries[key]
	switch {
	case e == nil:
		e = &treeEntry{node: &Node{Name: pl.Written, Reported: reported(pl)}, placeholder: action != ActionRun}
		b.entries[key] = e
		b.link(e, pl)
	case e.placeholder && action == ActionRun:
		// Its run says where it sits; the guess it was made by goes.
		b.unlink(e)
		e.node.Name, e.node.Reported = pl.Written, reported(pl)
		e.placeholder = false
		b.link(e, pl)
	}
	e.reported = true
	return e
}

// reported is pl's own level when it differs from the written one.
func reported(pl protocol.Placement) string {
	if pl.Own == pl.Written {
		return ""
	}
	return pl.Own
}

// link hangs e under its parent, making a placeholder for a parent not seen.
// A top-level node of a later instance is the external twin.
func (b *treeBuild) link(e *treeEntry, pl protocol.Placement) {
	if pl.Parent == "" {
		e.parent = nil
		if pl.Instance >= 2 {
			e.node.Variant, e.node.External = pl.Instance, true
		}
		b.pkg.Nodes = append(b.pkg.Nodes, e.node)
		return
	}
	key := treeKey{pl.Instance, pl.Parent}
	parent := b.entries[key]
	if parent == nil {
		guess := protocol.Guess(pl.Parent, pl.Instance)
		parent = &treeEntry{node: &Node{Name: guess.Written}, placeholder: true}
		b.entries[key] = parent
		b.link(parent, guess)
	}
	e.parent = parent
	parent.node.Children = append(parent.node.Children, e.node)
}

func (b *treeBuild) unlink(e *treeEntry) {
	same := func(n *Node) bool { return n == e.node }
	if e.parent == nil {
		b.pkg.Nodes = slices.DeleteFunc(b.pkg.Nodes, same)
		return
	}
	e.parent.node.Children = slices.DeleteFunc(e.parent.node.Children, same)
}

// prune drops placeholders left without an event or a child: guesses a
// later run replaced.
func (b *treeBuild) prune() {
	empty := map[*Node]bool{}
	for _, e := range b.entries {
		if e.placeholder && !e.reported {
			empty[e.node] = true
		}
	}
	if len(empty) > 0 {
		b.pkg.Nodes = pruneNodes(b.pkg.Nodes, empty)
	}
}

func pruneNodes(nodes []*Node, empty map[*Node]bool) []*Node {
	return slices.DeleteFunc(nodes, func(n *Node) bool {
		n.Children = pruneNodes(n.Children, empty)
		return empty[n] && len(n.Children) == 0
	})
}

// underBenchmark reports whether e or a node above it is a benchmark.
func (e *treeEntry) underBenchmark() bool {
	for ; e != nil; e = e.parent {
		if isBenchmarkName(e.node.Name) {
			return true
		}
	}
	return false
}

// declare gives each node what source declares at its written levels.
func declare(nodes []*Node, levels []string, lookup func([]string) Declaration) {
	for _, n := range nodes {
		own := append(slices.Clip(levels), n.Name)
		d := lookup(own)
		n.Vocab, n.SourceLabel = d.Vocab, d.Label
		declare(n.Children, own, lookup)
	}
}

// FailBranches fails every node a failure beneath it reached, and the package
// when any node failed, as go test fails a test with its subtest. A stream
// composed from several runs can hold a parent's pass over a child's newer
// fail; every renderer and the exit code then read the same verdict. A skip
// stays where it was reported.
func FailBranches(pkg *Package) {
	for _, n := range pkg.Nodes {
		if failBranch(n) {
			pkg.Status = StatusFail
		}
	}
}

func failBranch(n *Node) bool {
	failed := n.Status == StatusFail
	for _, c := range n.Children {
		if failBranch(c) {
			failed = true
		}
	}
	if failed {
		n.Status = StatusFail
	}
	return failed
}

// isBenchmarkName reports whether name is a Go benchmark identifier:
// "Benchmark" followed by nothing or a non-lowercase character — mirroring
// the stdlib's TestXxx/BenchmarkXxx convention. This is boundary-aware so
// that names like "Benchmarking_the_new_endpoint" (lowercase continuation)
// are not mistaken for benchmark identifiers.
func isBenchmarkName(name string) bool {
	rest, ok := strings.CutPrefix(name, protocol.PrefixBenchmark)
	if !ok {
		return false
	}
	if rest == "" {
		return true
	}
	r, _ := utf8.DecodeRuneInString(rest)
	return !unicode.IsLower(r)
}

// BenchLine is a go test benchmark result line, read whole:
//
//	BenchmarkFoo/sub-8   	    1201	     985.2 ns/op	   4.500 widgets/op	      24 B/op	       3 allocs/op
//
// After the name and the iteration count come "<value> <unit>" pairs in
// whatever order go test prints them: ns/op first, then MB/s and the metrics
// the benchmark reported, then B/op and allocs/op when memory is reported.
// The three the renderers know have fields of their own; the rest are Metrics.
type BenchLine struct {
	Name        string // the benchmark's full name without the GOMAXPROCS suffix
	Procs       int    // the GOMAXPROCS suffix, 0 when the line carries none
	Iterations  int
	NsPerOp     float64
	BytesPerOp  int64
	AllocsPerOp int64
	Metrics     map[string]float64
}

var benchNameRe = regexp.MustCompile(`^(Benchmark\S*?)(?:-(\d+))?$`)

// ParseBenchLine reads a benchmark result line. A line is one only when it
// starts with a benchmark name, carries an iteration count and reports ns/op.
func ParseBenchLine(line string) (BenchLine, bool) {
	fields := strings.Fields(line)
	if len(fields) < 4 || len(fields)%2 != 0 {
		return BenchLine{}, false
	}
	m := benchNameRe.FindStringSubmatch(fields[0])
	if m == nil {
		return BenchLine{}, false
	}
	iters, err := strconv.Atoi(fields[1])
	if err != nil {
		return BenchLine{}, false
	}
	out := BenchLine{Name: m[1], Iterations: iters}
	if m[2] != "" {
		out.Procs, _ = strconv.Atoi(m[2])
	}
	sawNs := false
	for k := 2; k < len(fields); k += 2 {
		value, err := strconv.ParseFloat(fields[k], 64)
		if err != nil {
			return BenchLine{}, false
		}
		switch unit := fields[k+1]; unit {
		case "ns/op":
			out.NsPerOp, sawNs = value, true
		case "B/op":
			out.BytesPerOp = int64(value)
		case "allocs/op":
			out.AllocsPerOp = int64(value)
		default:
			if out.Metrics == nil {
				out.Metrics = map[string]float64{}
			}
			out.Metrics[unit] = value
		}
	}
	if !sawNs {
		return BenchLine{}, false
	}
	return out, true
}

// benchScan finds a benchmark's result line in output that arrives in
// chunks. test2json's output events are not line-aligned: under real pipe
// timing a result line can arrive split mid-token across two of them, so
// only whole lines are read, and what follows the last newline waits for the
// next chunk. Each chunk is read once.
type benchScan struct {
	partial string
	found   bool
}

// read takes the next chunk of the node's output.
func (b *benchScan) read(node *Node, chunk string) {
	if b.found {
		return
	}
	b.partial += chunk
	for !b.found {
		line, rest, whole := strings.Cut(b.partial, "\n")
		if !whole {
			return
		}
		b.partial = rest
		b.parse(node, line)
	}
}

// end reads what the output left behind its last newline.
func (b *benchScan) end(node *Node) {
	if !b.found {
		b.parse(node, b.partial)
	}
}

func (b *benchScan) parse(node *Node, line string) {
	result, ok := ParseBenchLine(line)
	if !ok {
		return
	}
	b.found = true
	node.Iterations = result.Iterations
	node.NsPerOp = result.NsPerOp
	node.BytesPerOp = result.BytesPerOp
	node.AllocsPerOp = result.AllocsPerOp
	node.Metrics = result.Metrics
	node.Procs = result.Procs
	// go test's own -json encoder never emits a "pass" event for a
	// benchmark (see ActionBench doc comment); reaching a parsed ns/op line
	// is the only success signal a real benchmark run ever produces.
	if node.Status == StatusNone {
		node.Status = StatusPass
	}
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
	if n.Kind == KindBenchmark && len(n.Children) == 0 {
		s.Benchmarks++
		return
	}
	if n.Kind == KindFuzz {
		// One verdict per target; its seeds are evidence, not behaviors.
		s.Fuzzers++
		countVerdict(n, s)
		return
	}
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
		countVerdict(n, s)
	}
	for _, c := range n.Children {
		collectStats(c, s, inStdlib)
	}
}

// countVerdict counts a leaf or a fuzz target by its verdict; one the stream
// gave none counts as not run, so the buckets add up to what was counted.
func countVerdict(n *Node, s *Stats) {
	switch {
	case n.NotRun || n.Status == StatusNone:
		s.NotRun++
	case n.Status == StatusPass:
		s.Passed++
	case n.Status == StatusFail:
		s.Failed++
	case n.Status == StatusSkip:
		s.Skipped++
	}
}

// noVerdict reports a leaf the stream gave no verdict: it counts as not run
// and shows no duration.
func noVerdict(n *Node) bool {
	return n.NotRun || (len(n.Children) == 0 && n.Status == StatusNone)
}

func classify(n *Node, topLevel bool) {
	if n.Kind == KindFuzz {
		return // shaped by attachFuzzWrappers, children included
	}
	name := n.Name

	if topLevel {
		hasTestPrefix := strings.HasPrefix(name, "Test")
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
		case !hasTestPrefix && isBenchmarkName(raw):
			// A bare top-level Benchmark* node (no enclosing TestSuite),
			// e.g. from a plain go test -bench=. -json stream fed via
			// `gotest spec --input`. Names that started with "Test" (e.g.
			// "TestBenchmarkFoo", a legitimate stdlib test) must never
			// reach this branch — hasTestPrefix guards against that.
			n.Kind = KindBenchmark
			n.Display = strings.TrimPrefix(raw, protocol.PrefixBenchmark)
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
		case isBenchmarkName(name):
			n.Kind = KindBenchmark
			n.Display = strings.TrimPrefix(name, protocol.PrefixBenchmark)
		case strings.HasPrefix(name, "Test"):
			n.Kind = KindMethod
			n.Display = strings.TrimPrefix(strings.TrimPrefix(name, "Test"), "_")
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
			label := n.SourceLabel
			if label == "" {
				label = strings.ReplaceAll(name, "_", " ")
			}
			n.Display = n.Vocab.Apply(label)
		}
	}

	if n.Display == "" {
		n.Display = n.Name
	}

	for _, c := range n.Children {
		if n.Kind == KindBenchmark {
			classifySubBenchmark(c)
		} else {
			classify(c, false)
		}
	}
}

// classifySubBenchmark shapes a node b.Run opened under a benchmark: a
// benchmark itself, shown under the name it was given, whatever it looks like.
func classifySubBenchmark(n *Node) {
	n.Kind = KindBenchmark
	n.Display = n.Name
	for _, c := range n.Children {
		classifySubBenchmark(c)
	}
}

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

// attachFuzzWrappers re-homes every generated Fuzz<Suite>_<Method> wrapper
// under its suite: the wrapper is a top-level test only because Go's engine
// needs one symbol per target, while the specification it belongs to is
// the suite's. A suite that did not run (a -run filter, a fuzz-only
// package) is synthesized so the target still has a home; its verdict is
// its children's.
func attachFuzzWrappers(pkg *Package) {
	suites := map[string]*Node{}
	for _, n := range pkg.Nodes {
		if strings.HasSuffix(n.Name, protocol.SuffixTestSuite) && strings.HasPrefix(n.Name, "Test") && n.Variant < 2 {
			suites[n.Name] = n
		}
	}
	var kept []*Node
	var synthesized []*Node
	for _, n := range pkg.Nodes {
		suiteType, method, ok := protocol.SplitFuzzWrapper(n.Name)
		if !ok || n.Variant >= 2 {
			kept = append(kept, n)
			continue
		}
		suiteName := "Test" + suiteType
		suite := suites[suiteName]
		if suite == nil {
			suite = &Node{Name: suiteName, Status: StatusPass, Start: n.Start, End: n.End}
			suites[suiteName] = suite
			synthesized = append(synthesized, suite)
			kept = append(kept, suite)
		}
		if suite.synthesized() && n.Status == StatusFail {
			suite.Status = StatusFail
		}
		n.Kind = KindFuzz
		n.Name = method
		n.Display = strings.TrimPrefix(method, protocol.PrefixFuzz)
		for _, c := range n.Children {
			c.Kind = KindBlock
			c.Display = c.Name
			if rest, ok := strings.CutPrefix(c.Name, "seed#"); ok {
				if i, err := strconv.Atoi(rest); err == nil {
					c.Display = fmt.Sprintf("seed #%d", i+1)
				}
			}
		}
		suite.Children = append(suite.Children, n)
	}
	for _, suite := range synthesized {
		for _, c := range suite.Children {
			if c.Status == StatusFail {
				suite.Status = StatusFail
			}
		}
	}
	pkg.Nodes = kept
}

// synthesized reports whether the node was created by attachFuzzWrappers
// rather than by an event: it has no start time of its own.
func (n *Node) synthesized() bool { return n.Start.IsZero() && n.Duration == 0 && len(n.Output) == 0 }
