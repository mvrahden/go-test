package gotestspec

import (
	"encoding/json"
	"io"

	"github.com/mvrahden/go-test/internal/about"
)

// jsonRoot is the document of spec and summary --format=json. Version opens
// it: the CLI that produced the document, so a consumer streaming a large
// tree can gate on the producer before reading the rest.
type jsonRoot struct {
	Version  string        `json:"version"`
	Packages []jsonPackage `json:"packages"`
	Stats    jsonStats     `json:"stats"`
	// Groups is what each named ExclusiveGroup took; absent without one.
	Groups []jsonGroup `json:"groups,omitempty"`
}

type jsonGroup struct {
	Name   string `json:"name"`
	Suites int    `json:"suites"`
	// Wall is seconds from the group's first suite start to its last end.
	Wall float64 `json:"wall"`
}

type jsonPackage struct {
	Path string `json:"path"`
	// Dir is the package's directory, when its specification was read from
	// source (--static, --declared).
	Dir string `json:"dir,omitempty"`
	// RunLevel marks a package the run booked for itself rather than for
	// an import path, as its events did.
	RunLevel bool       `json:"runLevel,omitempty"`
	Status   string     `json:"status"`
	Duration float64    `json:"duration"`
	Nodes    []jsonNode `json:"nodes"`
	Output   []string   `json:"output"`
	// Stats counts this package alone; present under WithPackageStats.
	Stats *jsonStats `json:"stats,omitempty"`
}

type jsonNode struct {
	Name     string  `json:"name"`
	Display  string  `json:"display"`
	Kind     string  `json:"kind"`
	Vocab    string  `json:"vocab,omitempty"`
	Status   string  `json:"status"`
	Duration float64 `json:"duration"`
	Focused  bool    `json:"focused"`
	Excluded bool    `json:"excluded"`
	External bool    `json:"external"`
	// Incomplete says the children are a floor, not the whole list. Only a
	// statically read tree can set it, so it is absent from a rendered run.
	Incomplete bool `json:"incomplete,omitempty"`
	Variant    int  `json:"variant,omitempty"`
	// File (relative to the package's dir) and Line say where the source
	// declares the node; absent for a node only a run revealed.
	File        string     `json:"file,omitempty"`
	Line        int        `json:"line,omitempty"`
	Output      []string   `json:"output"`
	Children    []jsonNode `json:"children"`
	Iterations  int        `json:"iterations,omitempty"`
	NsPerOp     float64    `json:"ns_per_op,omitempty"`
	BytesPerOp  int64      `json:"bytes_per_op,omitempty"`
	AllocsPerOp int64      `json:"allocs_per_op,omitempty"`
	// Metrics are the result line's other units: MB/s and reported metrics.
	Metrics map[string]float64 `json:"metrics,omitempty"`
}

type jsonStats struct {
	Suites     int `json:"suites"`
	Behaviors  int `json:"behaviors"`
	Tests      int `json:"tests"`
	Benchmarks int `json:"benchmarks"`
	Fuzzers    int `json:"fuzzers"`
	Passed     int `json:"passed"`
	Failed     int `json:"failed"`
	Skipped    int `json:"skipped"`
	// FailedPackages carries package-level verdicts (build failures, deaths
	// outside any test); the packages array shows which via status "fail".
	FailedPackages int `json:"failedPackages,omitempty"`
	// NotRun counts declared behaviors the stream did not report (--declared).
	NotRun int `json:"notRun,omitempty"`
}

func RenderJSON(w io.Writer, packages []*Package, opts ...RenderOption) {
	var cfg renderConfig
	for _, o := range opts {
		o(&cfg)
	}
	packages, _ = cfg.narrow(packages)
	stats := cfg.stats(packages)

	root := jsonRoot{
		Version:  about.ResolvedVersion(),
		Packages: []jsonPackage{},
		Stats:    jsonStats(stats),
	}

	// Every package stays, with its stats: hiding leaves out rows, never a
	// count. One whose rows are all hidden keeps no node.
	for _, pkg := range packages {
		output := pkg.Output
		if output == nil || cfg.hidden[StatusFail] {
			// The failure a package carries on its own is a row the
			// other formats leave out under --hide=fail.
			output = []string{}
		}
		jp := jsonPackage{
			Path:     pkg.Path,
			Dir:      pkg.Dir,
			RunLevel: pkg.RunLevel,
			Status:   statusString(pkg.Status),
			Duration: PackageDuration(pkg).Seconds(),
			Nodes:    cfg.convertNodes(pkg.Nodes),
			Output:   output,
		}
		if cfg.packageStats {
			own := jsonStats(cfg.stats([]*Package{pkg}))
			jp.Stats = &own
		}
		root.Packages = append(root.Packages, jp)
	}

	for _, g := range cfg.groups {
		root.Groups = append(root.Groups, jsonGroup{Name: g.Name, Suites: g.Suites, Wall: g.Wall.Seconds()})
	}

	enc := json.NewEncoder(w)
	_ = enc.Encode(root)
}

func (c *renderConfig) convertNodes(all []*Node) []jsonNode {
	nodes := c.visible(all)
	if len(nodes) == 0 {
		return []jsonNode{}
	}
	result := make([]jsonNode, len(nodes))
	for i, n := range nodes {
		result[i] = jsonNode{
			Name:        n.Name,
			Display:     n.Display,
			Kind:        kindString(n.Kind),
			Vocab:       vocabString(n.Vocab),
			Status:      statusString(n.Status),
			Duration:    EffectiveDuration(n).Seconds(),
			Focused:     n.Focused,
			Excluded:    n.Excluded,
			External:    n.External,
			Incomplete:  n.Incomplete,
			Variant:     n.Variant,
			File:        n.File,
			Line:        n.Line,
			Output:      n.Output,
			Children:    c.convertNodes(n.Children),
			Iterations:  n.Iterations,
			NsPerOp:     n.NsPerOp,
			BytesPerOp:  n.BytesPerOp,
			AllocsPerOp: n.AllocsPerOp,
			Metrics:     n.Metrics,
		}
		if result[i].Output == nil {
			result[i].Output = []string{}
		}
		if noVerdict(n) {
			result[i].Duration = 0 // a row without a verdict took no time it can show
		}
	}
	return result
}

func statusString(s Status) string {
	switch s {
	case StatusPass:
		return "pass"
	case StatusFail:
		return "fail"
	case StatusSkip:
		return "skip"
	default:
		return "none"
	}
}

func vocabString(v Vocab) string {
	switch v {
	case VocabWhen:
		return "when"
	case VocabIt:
		return "it"
	case VocabEach:
		return "each"
	default:
		return ""
	}
}

func kindString(k NodeKind) string {
	switch k {
	case KindFixture:
		return "fixture"
	case KindSuite:
		return "suite"
	case KindMethod:
		return "method"
	case KindBlock:
		return "block"
	case KindTest:
		return "test"
	case KindBenchmark:
		return "benchmark"
	case KindFuzz:
		return "fuzz"
	default:
		return "unknown"
	}
}
