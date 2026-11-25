package gotestaudit

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/mvrahden/go-test/internal/about"
)

// Compare audits head against base. It reads only the two records.
func Compare(base, head *Record) Document {
	doc := Document{
		Version:    about.ResolvedVersion(),
		Schema:     Schema,
		Base:       Side{Version: base.Version, Provenance: base.Provenance, Patterns: base.Patterns},
		Head:       Side{Version: head.Version, Provenance: head.Provenance, Patterns: head.Patterns},
		Modules:    modulesOf(base, head),
		ModuleDirs: moduleDirsOf(head, base),
	}
	if !slices.Equal(base.Patterns, head.Patterns) {
		doc.Notes = append(doc.Notes, fmt.Sprintf("the records name other packages: base %s, head %s",
			strings.Join(base.Patterns, " "), strings.Join(head.Patterns, " ")))
	}
	m := &matcher{}
	bRoots, hRoots := m.packages(base, head, &doc)
	m.moved(bRoots, hRoots)
	doc.Notes = append(doc.Notes, m.notes...)

	c := &collector{doc: &doc}
	c.pairs(bRoots)
	c.unmatched(bRoots, ChangeRemoved)
	c.unmatched(hRoots, ChangeAdded)
	c.suppressions(base, head)
	c.snapshots(base, head)
	doc.finish()
	return doc
}

// packages pairs the packages both records can read and their suites, and
// returns every suite node of each side. A package one side cannot read is
// compared on neither.
func (m *matcher) packages(base, head *Record, doc *Document) (bRoots, hRoots []*node) {
	heads := map[string]*Package{}
	for i := range head.Packages {
		heads[head.Packages[i].Path] = &head.Packages[i]
	}
	bases := map[string]bool{}
	for i := range base.Packages {
		bp := &base.Packages[i]
		bases[bp.Path] = true
		hp := heads[bp.Path]
		bn := suiteNodes(bp)
		switch {
		case len(bp.Unreadable) > 0:
			doc.Incomplete = append(doc.Incomplete, bp.Path+": the base cannot be read: "+bp.Unreadable[0])
			skipAll(bn)
			if hp != nil {
				hn := suiteNodes(hp)
				skipAll(hn)
				hRoots = append(hRoots, hn...)
			}
		case hp != nil && len(hp.Unreadable) > 0:
			doc.Incomplete = append(doc.Incomplete, bp.Path+": the head cannot be read: "+hp.Unreadable[0])
			skipAll(bn)
		case hp != nil:
			hn := suiteNodes(hp)
			m.level(bn, hn)
			hRoots = append(hRoots, hn...)
		}
		bRoots = append(bRoots, bn...)
	}
	for i := range head.Packages {
		hp := &head.Packages[i]
		if bases[hp.Path] {
			continue
		}
		if len(hp.Unreadable) > 0 {
			doc.Incomplete = append(doc.Incomplete, hp.Path+": the head cannot be read: "+hp.Unreadable[0])
			continue
		}
		hRoots = append(hRoots, suiteNodes(hp)...)
	}
	return bRoots, hRoots
}

// moduleDirsOf are the records' module directories, the first record's
// where two name one module.
func moduleDirsOf(records ...*Record) []ModuleDir {
	seen := map[string]bool{}
	var out []ModuleDir
	for _, r := range records {
		for _, m := range r.ModuleDirs {
			if !seen[m.Path] {
				seen[m.Path] = true
				out = append(out, m)
			}
		}
	}
	slices.SortFunc(out, func(a, b ModuleDir) int { return cmp.Compare(a.Path, b.Path) })
	return out
}

func modulesOf(records ...*Record) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range records {
		for _, p := range r.Packages {
			if p.Module != "" && !seen[p.Module] {
				seen[p.Module] = true
				out = append(out, p.Module)
			}
		}
	}
	slices.Sort(out)
	return out
}

// collector turns the matched nodes into the document's lists.
type collector struct{ doc *Document }

func (c *collector) find(class, pkg, subject, base, head string, loc Location) {
	f := Finding{Class: class, Weakens: Weakens(class), Package: pkg, Subject: subject, Base: base, Head: head}
	if loc.File != "" {
		l := loc
		f.Location = &l
	}
	c.doc.Findings = append(c.doc.Findings, f)
}

// at names the behavior each side has for the finding just found: its
// labels, its place and its test, the head's where it has one.
func (c *collector) at(b, h *node) {
	f := &c.doc.Findings[len(c.doc.Findings)-1]
	if b != nil {
		f.basePath, f.Module = b.path, b.mod
		l := b.loc
		f.BaseLocation = &l
		f.Selector = selectorOf(b.names, !b.bench)
	}
	if h != nil {
		f.headPath, f.Module = h.path, h.mod
		l := h.loc
		f.HeadLocation = &l
		f.Selector = selectorOf(h.names, !h.bench)
	}
}

// selector is the test of the side a change has, the head's first.
func selector(b, h *node) Selector {
	if h != nil {
		return selectorOf(h.names, !h.bench)
	}
	return selectorOf(b.names, !b.bench)
}

func place(n *node) *Place { return &Place{Path: n.path, Location: n.loc} }

func subject(n *node) string { return strings.Join(n.path, " / ") }

// pairs reports each base node matched to a head node and what it lost.
func (c *collector) pairs(roots []*node) {
	var walk func([]*node)
	walk = func(ns []*node) {
		for _, b := range ns {
			if h := b.match; h != nil {
				c.pair(b, h)
			}
			walk(b.kids)
		}
	}
	walk(roots)
}

func (c *collector) pair(b, h *node) {
	switch {
	case b.how != "":
		c.doc.Behaviors = append(c.doc.Behaviors, Change{Change: b.how, Kind: h.kind, Package: h.pkg, Module: h.mod, Base: place(b), Head: place(h), Selector: selector(b, h)})
	case h.kind != KindSuite && (b.hash != h.hash || h.whole && b.tree != h.tree):
		c.doc.Behaviors = append(c.doc.Behaviors, Change{Change: ChangeChanged, Kind: h.kind, Package: h.pkg, Module: h.mod, Base: place(b), Head: place(h), Selector: selector(b, h)})
		c.find(ClassChanged, h.pkg, subject(h), b.hash, h.hash, h.loc)
		c.at(b, h)
	}
	if h.suite != nil {
		c.suitePair(b, h)
		return
	}
	if h.b.Excluded && !b.b.Excluded {
		c.find(ClassExcluded, h.pkg, subject(h), b.b.Name, h.b.Name, h.loc)
		c.at(b, h)
	}
	if h.b.Focused && !b.b.Focused {
		c.find(ClassFocus, h.pkg, subject(h), b.b.Name, h.b.Name, h.loc)
		c.at(b, h)
	}
	if h.b.Assertions < b.b.Assertions {
		c.find(ClassAssertions, h.pkg, subject(h), fmt.Sprint(b.b.Assertions), fmt.Sprint(h.b.Assertions), h.loc)
		c.at(b, h)
	}
	if h.b.AssertsBy && !b.b.AssertsBy {
		c.find(ClassAssertsBy, h.pkg, subject(h), "no AssertsBy", "AssertsBy", h.loc)
		c.at(b, h)
	}
}

func (c *collector) suitePair(bn, n *node) {
	b, h := bn.suite, n.suite
	if h.Excluded && !b.Excluded {
		c.find(ClassExcluded, n.pkg, subject(n), b.Name, h.Name, n.loc)
		c.at(bn, n)
	}
	if h.Focused && !b.Focused {
		c.find(ClassFocus, n.pkg, subject(n), b.Name, h.Name, n.loc)
		c.at(bn, n)
	}
	if h.Guard && !b.Guard {
		c.find(ClassGuard, n.pkg, subject(n), "no SuiteGuard", "SuiteGuard", n.loc)
		c.at(bn, n)
	}
	for _, d := range []struct{ name, base, head string }{
		{"Timeout", b.Budget.Timeout, h.Budget.Timeout},
		{"SetupTimeout", b.Budget.SetupTimeout, h.Budget.SetupTimeout},
	} {
		raised, known := budgetLess(d.base, d.head)
		if raised || (!known && d.base != BudgetUnknown && d.head == BudgetUnknown) {
			c.find(ClassBudget, n.pkg, subject(n), d.name+" "+d.base, d.name+" "+d.head, n.loc)
			c.at(bn, n)
		}
	}
}

// unmatched reports the topmost nodes of one side with no partner: what the
// head removed or added. An added one that is focused or excluded is found
// as such.
func (c *collector) unmatched(roots []*node, change string) {
	var walk func([]*node)
	walk = func(ns []*node) {
		for _, n := range ns {
			if n.skipped {
				continue
			}
			if n.match == nil && (n.parent == nil || n.parent.match != nil) {
				ch := Change{Change: change, Kind: n.kind, Package: n.pkg, Module: n.mod, Behaviors: n.leaves(), Selector: selector(nil, n)}
				if change == ChangeRemoved {
					ch.Base = place(n)
					c.find(ClassRemoved, n.pkg, subject(n), n.kind, "", n.loc)
					c.at(n, nil)
				} else {
					ch.Head = place(n)
					c.marked(n)
				}
				c.doc.Behaviors = append(c.doc.Behaviors, ch)
			}
			walk(n.kids)
		}
	}
	walk(roots)
}

// marked finds the F_ and X_ an added subtree carries.
func (c *collector) marked(n *node) {
	focused, excluded, name := false, false, ""
	switch {
	case n.suite != nil:
		focused, excluded, name = n.suite.Focused, n.suite.Excluded, n.suite.Name
	case n.b != nil:
		focused, excluded, name = n.b.Focused, n.b.Excluded, n.b.Name
	}
	if excluded {
		c.find(ClassExcluded, n.pkg, subject(n), "", name, n.loc)
		c.at(nil, n)
	}
	if focused {
		c.find(ClassFocus, n.pkg, subject(n), "", name, n.loc)
		c.at(nil, n)
	}
	for _, k := range n.kids {
		if k.match == nil {
			c.marked(k)
		}
	}
}

// suppressions finds each comment the head adds, matched by module, file,
// rules and reason, so one that only moved is no finding. Where one comment
// of each side is left with the same file and rules, it only changed its
// reason: information, not a weakening.
func (c *collector) suppressions(base, head *Record) {
	if unread(base, "suppressions") || unread(head, "suppressions") {
		c.doc.Incomplete = append(c.doc.Incomplete, "suppressions: a record could not list them")
		return
	}
	rules := func(s *Suppression) string { return s.Module + "\x00" + s.File + "\x00" + strings.Join(s.Rules, ",") }
	key := func(s *Suppression) string { return rules(s) + "\x00" + s.Reason }
	baseList, headList := withMutateIgnores(base), withMutateIgnores(head)
	have := map[string]int{}
	for i := range baseList {
		have[key(&baseList[i])]++
	}
	var added []*Suppression
	for i := range headList {
		s := &headList[i]
		if k := key(s); have[k] > 0 {
			have[k]--
			continue
		}
		added = append(added, s)
	}
	left := map[string][]*Suppression{}
	for i := range baseList {
		s := &baseList[i]
		if k := key(s); have[k] > 0 {
			have[k]--
			left[rules(s)] = append(left[rules(s)], s)
		}
	}
	addedRules := map[string]int{}
	for _, s := range added {
		addedRules[rules(s)]++
	}
	for _, s := range added {
		loc := Location{File: s.File, Line: s.Line, EndLine: s.Line}
		head := loc
		if was := left[rules(s)]; len(was) == 1 && addedRules[rules(s)] == 1 {
			base := Location{File: was[0].File, Line: was[0].Line, EndLine: was[0].Line}
			c.doc.Findings = append(c.doc.Findings, Finding{Class: ClassSuppression, Module: s.Module, Subject: s.File, Base: suppressionText(was[0]), Head: suppressionText(s),
				Location: &loc, BaseLocation: &base, HeadLocation: &head})
			continue
		}
		c.find(ClassSuppression, "", s.File, "", suppressionText(s), loc)
		f := &c.doc.Findings[len(c.doc.Findings)-1]
		f.Module, f.HeadLocation = s.Module, &head
	}
}

// mutateIgnoreRule is the rule a //gotest:mutate-ignore comment is
// compared under, beside the nolint comments.
const mutateIgnoreRule = "gotest:mutate-ignore"

// withMutateIgnores is a record's suppressions and its mutate-ignore
// comments, compared alike: one added is a suppression finding.
func withMutateIgnores(r *Record) []Suppression {
	out := slices.Clone(r.Suppressions)
	for _, ig := range r.MutateIgnores {
		out = append(out, Suppression{Module: ig.Module, File: ig.File, Line: ig.Line, Rules: []string{mutateIgnoreRule}, Reason: ig.Reason})
	}
	return out
}

func suppressionText(s *Suppression) string {
	if len(s.Rules) == 1 && s.Rules[0] == mutateIgnoreRule {
		return strings.TrimSpace("//" + mutateIgnoreRule + " " + s.Reason)
	}
	text := "//nolint:" + strings.Join(s.Rules, ",")
	if s.Reason != "" {
		text += " // " + s.Reason
	}
	return text
}

func unread(r *Record, what string) bool {
	for _, u := range r.Unread {
		if strings.HasPrefix(u, what+":") {
			return true
		}
	}
	return false
}

// snapshots finds each baseline the head rewrote or deleted.
func (c *collector) snapshots(base, head *Record) {
	heads := map[string]string{}
	for _, s := range head.Snapshots {
		heads[s.Module+"\x00"+s.File] = s.SHA256
	}
	for _, s := range base.Snapshots {
		sum, ok := heads[s.Module+"\x00"+s.File]
		file := Location{File: s.File}
		switch {
		case !ok:
			c.find(ClassSnapshot, "", s.File, s.SHA256[:12], "deleted", file)
		case sum != s.SHA256:
			c.find(ClassSnapshot, "", s.File, s.SHA256[:12], sum[:12], file)
			f := &c.doc.Findings[len(c.doc.Findings)-1]
			f.HeadLocation = &Location{File: s.File}
		default:
			continue
		}
		f := &c.doc.Findings[len(c.doc.Findings)-1]
		f.Module, f.BaseLocation = s.Module, &Location{File: s.File}
	}
}

// finish orders the lists and counts them.
func (d *Document) finish() {
	order := map[string]int{}
	for i, c := range Classes {
		order[c] = i
	}
	slices.SortStableFunc(d.Findings, func(a, b Finding) int {
		return cmp.Or(cmp.Compare(order[a.Class], order[b.Class]), cmp.Compare(a.Package, b.Package),
			cmp.Compare(a.Subject, b.Subject), cmp.Compare(a.Base, b.Base), cmp.Compare(a.Head, b.Head))
	})
	changeOrder := map[string]int{ChangeRemoved: 0, ChangeAdded: 1, ChangeReworded: 2, ChangeMoved: 3, ChangeChanged: 4}
	slices.SortStableFunc(d.Behaviors, func(a, b Change) int {
		return cmp.Or(cmp.Compare(changeOrder[a.Change], changeOrder[b.Change]), cmp.Compare(a.Package, b.Package),
			cmp.Compare(changePath(&a), changePath(&b)))
	})
	for i := range d.Findings {
		f := &d.Findings[i]
		f.ID = stableID(f.Class, f.Module, f.Package, f.Subject, f.Base, f.Head)
	}
	d.Summary = Summary{Classes: map[string]int{}, Incomplete: len(d.Incomplete)}
	for i := range d.Findings {
		f := &d.Findings[i]
		if f.Weakens || f.Class == ClassChanged {
			d.Summary.Classes[f.Class]++
		}
		if f.Weakens {
			d.Summary.Weakenings++
		} else {
			d.Summary.Informational++
		}
	}
	for i := range d.Behaviors {
		switch d.Behaviors[i].Change {
		case ChangeAdded:
			d.Summary.Added++
		case ChangeRemoved:
			d.Summary.Removed++
		case ChangeReworded:
			d.Summary.Reworded++
		case ChangeMoved:
			d.Summary.Moved++
		case ChangeChanged:
			d.Summary.Changed++
		}
	}
	d.link()
}

func changePath(c *Change) string {
	p := c.Head
	if p == nil {
		p = c.Base
	}
	return strings.Join(p.Path, "\x00")
}

func (d *Document) normalize() {
	if d.Findings == nil {
		d.Findings = []Finding{}
	}
	if d.Behaviors == nil {
		d.Behaviors = []Change{}
	}
	if d.Evidence == nil {
		d.Evidence = []Evidence{}
	}
	if d.Unproven == nil {
		d.Unproven = []Unproven{}
	}
	if d.Incomplete == nil {
		d.Incomplete = []string{}
	}
	if d.Notes == nil {
		d.Notes = []string{}
	}
	if d.Streams == nil {
		d.Streams = []Stream{}
	}
	if d.Modules == nil {
		d.Modules = []string{}
	}
	if d.ModuleDirs == nil {
		d.ModuleDirs = []ModuleDir{}
	}
	if d.Base.Patterns == nil {
		d.Base.Patterns = []string{}
	}
	if d.Head.Patterns == nil {
		d.Head.Patterns = []string{}
	}
}
