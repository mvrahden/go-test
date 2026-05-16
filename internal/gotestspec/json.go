package gotestspec

import (
	"encoding/json"
	"io"
)

type jsonRoot struct {
	Packages []jsonPackage `json:"packages"`
	Stats    jsonStats     `json:"stats"`
}

type jsonPackage struct {
	Path     string     `json:"path"`
	Status   string     `json:"status"`
	Duration float64    `json:"duration"`
	Nodes    []jsonNode `json:"nodes"`
}

type jsonNode struct {
	Name     string     `json:"name"`
	Display  string     `json:"display"`
	Kind     string     `json:"kind"`
	Status   string     `json:"status"`
	Duration float64    `json:"duration"`
	Focused  bool       `json:"focused"`
	Excluded bool       `json:"excluded"`
	External bool       `json:"external"`
	Variant  int        `json:"variant,omitempty"`
	Output   []string   `json:"output"`
	Children []jsonNode `json:"children"`
}

type jsonStats struct {
	Suites    int `json:"suites"`
	Behaviors int `json:"behaviors"`
	Tests     int `json:"tests"`
	Passed    int `json:"passed"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
}

func RenderJSON(w io.Writer, packages []*Package) {
	stats := CollectStats(packages)

	root := jsonRoot{
		Packages: make([]jsonPackage, len(packages)),
		Stats: jsonStats{
			Suites:    stats.Suites,
			Behaviors: stats.Behaviors,
			Tests:     stats.Tests,
			Passed:    stats.Passed,
			Failed:    stats.Failed,
			Skipped:   stats.Skipped,
		},
	}

	for i, pkg := range packages {
		root.Packages[i] = jsonPackage{
			Path:     pkg.Path,
			Status:   statusString(pkg.Status),
			Duration: pkg.Duration.Seconds(),
			Nodes:    convertNodes(pkg.Nodes),
		}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(root)
}

func convertNodes(nodes []*Node) []jsonNode {
	if len(nodes) == 0 {
		return []jsonNode{}
	}
	result := make([]jsonNode, len(nodes))
	for i, n := range nodes {
		result[i] = jsonNode{
			Name:     n.Name,
			Display:  n.Display,
			Kind:     kindString(n.Kind),
			Status:   statusString(n.Status),
			Duration: n.Duration.Seconds(),
			Focused:  n.Focused,
			Excluded: n.Excluded,
			External: n.External,
			Variant:  n.Variant,
			Output:   n.Output,
			Children: convertNodes(n.Children),
		}
		if result[i].Output == nil {
			result[i].Output = []string{}
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
	default:
		return "unknown"
	}
}
