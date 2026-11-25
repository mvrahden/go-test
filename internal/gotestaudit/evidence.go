package gotestaudit

import (
	"fmt"
	"slices"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestspec"
)

// StreamInput is one run's -json stream given as evidence.
type StreamInput struct {
	// File is where it was read from; empty for the audit's own run.
	File string
	Side string
	// Ran marks the run the audit made of the working tree itself.
	Ran    bool
	Events []gotestspec.TestEvent
	// Fresh says, of each package whose verdict the stream stamped with an
	// input hash, whether the head's tree still has that hash; nil when the
	// head is not a tree on disk, and the commit judges.
	Fresh map[string]bool
}

// rnode is a record's suite or behavior as the evidence reader walks it.
type rnode struct {
	name     string // the level go test reports for it
	kind     string
	excluded bool
	kids     []*rnode
}

// target is a behavior of a record: its levels from the suite down.
type target struct {
	chain          []*rnode
	external, twin bool
}

// recordIndex finds a record's suites and behaviors by package and labels.
type recordIndex struct {
	targets map[string]*target
	twins   gotestspec.DeclarationIndex
}

func pathKey(pkg string, path []string) string {
	return pkg + "\x00" + strings.Join(path, "\x00")
}

func indexRecord(r *Record) *recordIndex {
	idx := &recordIndex{targets: map[string]*target{}, twins: gotestspec.DeclarationIndex{}}
	for i := range r.Packages {
		p := &r.Packages[i]
		sides := map[string]int{}
		for j := range p.Suites {
			bit := 1
			if p.Suites[j].External {
				bit = 2
			}
			sides[p.Suites[j].Name] |= bit
		}
		for j := range p.Suites {
			s := &p.Suites[j]
			twin := sides[s.Name] == 3
			if twin {
				if idx.twins[p.Path] == nil {
					idx.twins[p.Path] = map[string]gotestspec.Declaration{}
				}
				idx.twins[p.Path][gotestspec.DeclKey("Test"+s.Name)] = gotestspec.Declaration{Twin: true}
			}
			root := &rnode{name: "Test" + s.Name, kind: KindSuite, excluded: s.Excluded}
			idx.add(p.Path, []string{s.Display}, []*rnode{root}, s.External, twin)
			root.kids = idx.behaviors(p.Path, []string{s.Display}, []*rnode{root}, s.Methods, s.External, twin)
		}
	}
	return idx
}

func (idx *recordIndex) add(pkg string, path []string, chain []*rnode, external, twin bool) {
	k := pathKey(pkg, path)
	if idx.targets[k] == nil {
		idx.targets[k] = &target{chain: slices.Clone(chain), external: external, twin: twin}
	}
}

func (idx *recordIndex) behaviors(pkg string, path []string, chain []*rnode, bs []Behavior, external, twin bool) []*rnode {
	out := make([]*rnode, 0, len(bs))
	for i := range bs {
		b := &bs[i]
		n := &rnode{name: b.Name, kind: b.Kind, excluded: b.Excluded}
		p := append(slices.Clip(path), b.Display)
		c := append(slices.Clip(chain), n)
		idx.add(pkg, p, c, external, twin)
		n.kids = idx.behaviors(pkg, p, c, b.Children, external, twin)
		out = append(out, n)
	}
	return out
}

// selector is the test go test runs for t, none under a benchmark or a
// fuzz target.
func (t *target) selector() Selector {
	names := make([]string, len(t.chain))
	runnable := true
	for i, r := range t.chain {
		names[i] = r.name
		runnable = runnable && r.kind != KindBenchmark && r.kind != KindFuzz
	}
	return selectorOf(names, runnable)
}

func (idx *recordIndex) find(pkg string, path []string) *target {
	return idx.targets[pathKey(pkg, path)]
}

// judgment is what one stream says of one behavior.
type judgment struct {
	verdict    string
	assertions *int
	vacuous    bool
	assertsBy  string
	skipReason string
	runs       int
	leaves     *Leaves
}

// streamTree is one stream read: its run and its packages.
type streamTree struct {
	stream Stream
	pkgs   map[string]*gotestspec.Package
	// byCommit is the commit rule's judgment, for a package without a hash.
	byCommit struct {
		current bool
		stale   string
	}
	fresh map[string]bool
	ran   bool
}

// current is currentFor without the reason.
func (st *streamTree) current(pkg string) bool {
	c, _ := st.currentFor(pkg)
	return c
}

// currentFor is whether the stream's verdicts of pkg judged the head's
// tree, and why not: by its input hash where the stream stamped one, else
// by commit.
func (st *streamTree) currentFor(pkg string) (bool, string) {
	if st.ran {
		return true, ""
	}
	if fresh, ok := st.fresh[pkg]; ok {
		if fresh {
			return true, ""
		}
		return false, "the inputs of " + pkg + " changed since the stream ran"
	}
	return st.byCommit.current, st.byCommit.stale
}

func readStream(in *StreamInput, idx *recordIndex, side *Side) streamTree {
	st := streamTree{stream: Stream{File: in.File, Side: in.Side, Ran: in.Ran}, pkgs: map[string]*gotestspec.Package{}}
	if run, ok := gotestspec.RunOf(in.Events); ok {
		m := run.Manifest
		st.stream.Run = &StreamRun{Version: m.Version, GoVersion: m.GoVersion, Args: m.Args, Commit: m.Commit, Dirty: m.Dirty, Agent: m.Agent, ExitCode: run.ExitCode}
	}
	st.byCommit.current, st.byCommit.stale = currency(st.stream.Run, side, in.Side, in.Ran)
	st.fresh, st.ran = in.Fresh, in.Ran
	st.stream.Current, st.stream.Stale = st.byCommit.current, st.byCommit.stale
	for _, p := range gotestspec.BuildTree(in.Events, gotestspec.WithDeclarations(idx.twins)) {
		st.pkgs[p.Path] = p
	}
	if len(st.fresh) > 0 && !in.Ran {
		st.stream.Current, st.stream.Stale = streamFreshness(st.fresh)
	}
	return st
}

// streamFreshness judges a stream by its packages' hashes: current when
// the head's tree has each, else stale, naming those it lacks.
func streamFreshness(fresh map[string]bool) (bool, string) {
	var changed []string
	for pkg, ok := range fresh {
		if !ok {
			changed = append(changed, pkg)
		}
	}
	if len(changed) == 0 {
		return true, ""
	}
	slices.Sort(changed)
	return false, "the inputs changed since the stream ran: " + strings.Join(changed, ", ")
}

// currency says whether a stream ran the side's tree: the audit's own run
// did; else its commit must be the side's and neither tree dirty.
func currency(run *StreamRun, side *Side, name string, ran bool) (current bool, stale string) {
	switch {
	case ran:
		return true, ""
	case run == nil:
		return false, "the stream has no manifest"
	case run.Commit == "":
		return false, "the stream names no commit"
	case side.Commit == "":
		return false, "the " + name + " names no commit"
	case !sameCommit(run.Commit, side.Commit):
		return false, fmt.Sprintf("another commit: the stream ran %s, the %s is %s", run.Commit, name, side.Commit)
	case run.Dirty == nil || *run.Dirty:
		return false, "the stream's tree had uncommitted changes"
	case side.Dirty == nil || *side.Dirty:
		return false, "the " + name + " has uncommitted changes"
	}
	return true, ""
}

// sameCommit compares two abbreviations of a commit, which git may cut to
// different lengths.
func sameCommit(a, b string) bool {
	return strings.HasPrefix(a, b) || strings.HasPrefix(b, a)
}

// judge is what st says of t in package pkg.
func (st *streamTree) judge(pkg string, t *target) judgment {
	p := st.pkgs[pkg]
	if p == nil {
		return judgment{verdict: VerdictNone}
	}
	var runs []judgment
	for _, top := range p.Nodes {
		if top.Name == t.chain[0].name && (!t.twin || top.External == t.external) {
			runs = append(runs, walk(top, t.chain))
		}
	}
	if len(runs) == 0 {
		return missing(t.chain, false, "")
	}
	return combine(runs)
}

// walk follows chain down from the stream node n of its first level.
func walk(n *gotestspec.Node, chain []*rnode) judgment {
	for i := 1; i < len(chain); i++ {
		c := childAt(n, chain[i].name)
		if c == nil {
			return missing(chain, n.Status == gotestspec.StatusSkip, n.SkipReason)
		}
		n = c
	}
	return found(n, chain[len(chain)-1])
}

// ownLevel is the level go test reported for n, testing's #01 included.
func ownLevel(n *gotestspec.Node) string {
	if n.Reported != "" {
		return n.Reported
	}
	return n.Name
}

func childAt(n *gotestspec.Node, name string) *gotestspec.Node {
	for _, c := range n.Children {
		if ownLevel(c) == name {
			return c
		}
	}
	return nil
}

// missing judges the last of chain, which the stream does not hold:
// skipped where the run skipped a level above or X_ excludes a level, else
// not run.
func missing(chain []*rnode, skippedAbove bool, reason string) judgment {
	excluded := false
	for _, r := range chain {
		excluded = excluded || r.excluded
	}
	j := judgment{verdict: VerdictNotRun}
	switch {
	case excluded:
		j.verdict, j.skipReason = VerdictSkip, "excluded by X_"
	case skippedAbove:
		j.verdict, j.skipReason = VerdictSkip, reason
	}
	last := chain[len(chain)-1]
	if len(last.kids) > 0 {
		j.leaves = &Leaves{}
		countMissing(last.kids, j.verdict == VerdictSkip, j.leaves)
	}
	return j
}

func countMissing(rs []*rnode, skipped bool, t *Leaves) {
	for _, r := range rs {
		s := skipped || r.excluded
		if len(r.kids) > 0 {
			countMissing(r.kids, s, t)
			continue
		}
		if s {
			t.Skipped++
		} else {
			t.NotRun++
		}
	}
}

func verdictOf(s gotestspec.Status) string {
	switch s {
	case gotestspec.StatusPass:
		return VerdictPass
	case gotestspec.StatusFail:
		return VerdictFail
	case gotestspec.StatusSkip:
		return VerdictSkip
	default:
		return VerdictNotRun
	}
}

// found judges the stream node n of the record node r.
func found(n *gotestspec.Node, r *rnode) judgment {
	j := judgment{verdict: verdictOf(n.Status), skipReason: n.SkipReason}
	if len(r.kids) == 0 && len(n.Children) == 0 {
		j.assertions, j.vacuous, j.assertsBy = n.Assertions, n.Vacuous(), n.AssertsBy
		return j
	}
	j.leaves = &Leaves{}
	sum, known := 0, true
	tally(r.kids, n.Children, n.Status == gotestspec.StatusSkip, j.leaves, &sum, &known)
	if known && j.leaves.Passed+j.leaves.Failed > 0 {
		j.assertions = &sum
	}
	j.vacuous = j.leaves.Vacuous > 0
	return j
}

// tally counts the leaves of the record nodes rs laid over the stream nodes
// ss: a leaf the stream lacks did not run, one only the stream has did.
func tally(rs []*rnode, ss []*gotestspec.Node, skipped bool, t *Leaves, sum *int, known *bool) {
	used := map[*gotestspec.Node]bool{}
	for _, r := range rs {
		var s *gotestspec.Node
		for _, c := range ss {
			if !used[c] && ownLevel(c) == r.name {
				s = c
				break
			}
		}
		if s == nil {
			countMissing([]*rnode{r}, skipped || r.excluded, t)
			continue
		}
		used[s] = true
		tally(r.kids, s.Children, skipped || s.Status == gotestspec.StatusSkip, t, sum, known)
		if len(r.kids) == 0 && len(s.Children) == 0 {
			countLeaf(s, t, sum, known)
		}
	}
	for _, c := range ss {
		if used[c] {
			continue
		}
		if len(c.Children) == 0 {
			countLeaf(c, t, sum, known)
			continue
		}
		tally(nil, c.Children, skipped, t, sum, known)
	}
}

func countLeaf(n *gotestspec.Node, t *Leaves, sum *int, known *bool) {
	switch n.Status {
	case gotestspec.StatusPass:
		t.Passed++
		if n.Vacuous() {
			t.Vacuous++
		}
	case gotestspec.StatusFail:
		t.Failed++
	case gotestspec.StatusSkip:
		t.Skipped++
		return
	default:
		t.NotRun++
		return
	}
	if n.Assertions == nil {
		*known = false
		return
	}
	*sum += *n.Assertions
}

// verdictRank orders verdicts for runs of one stream: the worst wins.
var verdictRank = map[string]int{VerdictFail: 0, VerdictPass: 1, VerdictSkip: 2, VerdictNotRun: 3, VerdictNone: 4}

// combine folds the runs of one behavior a stream repeated (-count).
func combine(runs []judgment) judgment {
	j := runs[0]
	if len(runs) == 1 {
		return j
	}
	j.runs = len(runs)
	for _, r := range runs[1:] {
		if verdictRank[r.verdict] < verdictRank[j.verdict] {
			j.verdict = r.verdict
		}
		if r.assertions != nil && (j.assertions == nil || *r.assertions < *j.assertions) {
			j.assertions = r.assertions
		}
		j.vacuous = j.vacuous || r.vacuous
		j.assertsBy = cmpOr(j.assertsBy, r.assertsBy)
		j.skipReason = cmpOr(j.skipReason, r.skipReason)
		if r.leaves != nil {
			if j.leaves == nil {
				j.leaves = &Leaves{}
			}
			l := *j.leaves
			l.Passed += r.leaves.Passed
			l.Failed += r.leaves.Failed
			l.Skipped += r.leaves.Skipped
			l.NotRun += r.leaves.NotRun
			l.Vacuous += r.leaves.Vacuous
			j.leaves = &l
		}
	}
	return j
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// best picks, of the streams' judgments, the one a reader trusts most: one
// that holds the package, a current stream before a stale one, then the
// first given.
func best(trees []streamTree, js []judgment, pkg string) int {
	pick := -1
	for i := range js {
		if js[i].verdict == VerdictNone {
			continue
		}
		if pick < 0 || trees[i].current(pkg) && !trees[pick].current(pkg) {
			pick = i
		}
	}
	return pick
}

// evidenceSubject is a behavior the document names, with why.
type evidenceSubject struct {
	pkg  string
	path []string
	kind string
	why  []string
}

// subjects are the behaviors the evidence speaks of, in document order:
// the changed ones, then the findings' subjects not among them.
func subjects(d *Document) []*evidenceSubject {
	var out []*evidenceSubject
	at := map[string]*evidenceSubject{}
	add := func(pkg string, path []string, kind, why string) {
		k := pathKey(pkg, path)
		s := at[k]
		if s == nil {
			s = &evidenceSubject{pkg: pkg, path: path, kind: kind}
			at[k] = s
			out = append(out, s)
		}
		if !slices.Contains(s.why, why) {
			s.why = append(s.why, why)
		}
	}
	for i := range d.Behaviors {
		c := &d.Behaviors[i]
		if c.Head != nil {
			add(c.Package, c.Head.Path, c.Kind, c.Change)
		}
	}
	for i := range d.Findings {
		f := &d.Findings[i]
		if f.headPath != nil && f.Package != "" {
			add(f.Package, f.headPath, "", f.Class)
		}
	}
	return out
}

// AddEvidence states, for each behavior the document names, what the
// streams of its side say, and counts it. base and head are the records
// compared.
func AddEvidence(d *Document, base, head *Record, inputs []StreamInput) {
	hIdx, bIdx := indexRecord(head), indexRecord(base)
	var heads, bases []streamTree
	var headAt []int
	for i := range inputs {
		in := &inputs[i]
		if in.Side == SideBase {
			bases = append(bases, readStream(in, bIdx, &d.Base))
			d.Streams = append(d.Streams, bases[len(bases)-1].stream)
			continue
		}
		heads = append(heads, readStream(in, hIdx, &d.Head))
		headAt = append(headAt, len(d.Streams))
		d.Streams = append(d.Streams, heads[len(heads)-1].stream)
	}
	if len(heads) > 0 {
		for _, s := range subjects(d) {
			d.Evidence = append(d.Evidence, evidenceFor(s, hIdx, heads, headAt))
		}
	}
	runtimeAssertions(d, bIdx, bases)
	d.countEvidence()
}

func evidenceFor(s *evidenceSubject, idx *recordIndex, trees []streamTree, at []int) Evidence {
	e := Evidence{ID: stableID("evidence", s.pkg, strings.Join(s.path, "\x00")), Package: s.pkg, Path: s.path, Kind: s.kind, Why: s.why, Verdict: VerdictNone, NeedsDecision: true}
	t := idx.find(s.pkg, s.path)
	if t == nil {
		return e
	}
	if e.Kind == "" {
		e.Kind = t.chain[len(t.chain)-1].kind
	}
	e.Selector = t.selector()
	js := make([]judgment, len(trees))
	for i := range trees {
		js[i] = trees[i].judge(s.pkg, t)
	}
	pick := best(trees, js, s.pkg)
	if pick < 0 {
		return e
	}
	j := js[pick]
	stream := at[pick]
	e.Verdict, e.Assertions, e.Vacuous, e.AssertsBy, e.SkipReason, e.Runs, e.Leaves = j.verdict, j.assertions, j.vacuous, j.assertsBy, j.skipReason, j.runs, j.leaves
	st := &trees[pick].stream
	e.Current, e.Stale = trees[pick].currentFor(s.pkg)
	e.Stream = &stream
	e.Source = st.File
	if st.Ran {
		e.Source = "--run"
	}
	if st.Run != nil {
		e.Commit = st.Run.Commit
	}
	e.NeedsDecision = e.Verdict == VerdictFail || e.Vacuous || e.notProven() || !e.Current
	for i := range js {
		if i != pick && js[i].verdict != VerdictNone {
			e.Others = append(e.Others, OtherVerdict{Stream: at[i], Verdict: js[i].verdict, Current: trees[i].current(s.pkg)})
		}
	}
	return e
}

// runtimeAssertions gives each assertions finding the counts the runs of
// each side reported.
func runtimeAssertions(d *Document, bIdx *recordIndex, bases []streamTree) {
	heads := map[string]*Evidence{}
	for i := range d.Evidence {
		heads[pathKey(d.Evidence[i].Package, d.Evidence[i].Path)] = &d.Evidence[i]
	}
	for i := range d.Findings {
		f := &d.Findings[i]
		if f.Class != ClassAssertions {
			continue
		}
		var rt Runtime
		if e := heads[pathKey(f.Package, f.headPath)]; e != nil && e.Verdict != VerdictNone {
			rt.Head = e.Assertions
		}
		if t := bIdx.find(f.Package, f.basePath); t != nil && len(bases) > 0 {
			js := make([]judgment, len(bases))
			for k := range bases {
				js[k] = bases[k].judge(f.Package, t)
			}
			if pick := best(bases, js, f.Package); pick >= 0 {
				rt.Base = js[pick].assertions
			}
		}
		if rt.Base != nil || rt.Head != nil {
			f.Runtime = &rt
		}
	}
}

// notProven reports an entry whose behavior, or a leaf beneath, did not run.
func (e *Evidence) notProven() bool {
	return e.Verdict == VerdictNotRun || e.Verdict == VerdictNone || e.Leaves != nil && e.Leaves.NotRun > 0
}

// countEvidence counts the evidence into the summary and its classes.
func (d *Document) countEvidence() {
	s := &d.Summary
	s.Evidence, s.Failing, s.NotRun, s.Vacuous, s.Stale = len(d.Evidence), 0, 0, 0, 0
	for i := range d.Evidence {
		e := &d.Evidence[i]
		if e.Verdict == VerdictFail {
			s.Failing++
		}
		if e.notProven() {
			s.NotRun++
		}
		if e.Vacuous {
			s.Vacuous++
		}
		if e.Stream != nil && !e.Current {
			s.Stale++
		}
	}
	setClass(s, ClassFailing, s.Failing)
	setClass(s, ClassNotRun, s.NotRun)
	setClass(s, ClassVacuous, s.Vacuous)
	setClass(s, ClassStale, s.Stale)
	d.normalize()
}

func setClass(s *Summary, class string, n int) {
	if s.Classes == nil {
		s.Classes = map[string]int{}
	}
	if n > 0 {
		s.Classes[class] = n
		return
	}
	delete(s.Classes, class)
}
