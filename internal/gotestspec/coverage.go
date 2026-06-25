package gotestspec

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

type CoverageReport struct {
	Total    float64
	Packages []PackageCoverage
}

type PackageCoverage struct {
	Path       string
	Percentage float64
	Covered    int
	Total      int
}

func ParseCoverageProfile(path string) (*CoverageReport, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseCoverageReader(f)
}

func parseCoverageReader(r io.Reader) (*CoverageReport, error) {
	type pkgAccum struct {
		covered int
		total   int
	}
	pkgs := map[string]*pkgAccum{}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "mode:") {
			continue
		}

		file, stmts, count, err := parseCoverageLine(line)
		if err != nil {
			continue
		}

		pkg := coveragePackage(file)
		acc := pkgs[pkg]
		if acc == nil {
			acc = &pkgAccum{}
			pkgs[pkg] = acc
		}
		acc.total += stmts
		if count > 0 {
			acc.covered += stmts
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	var totalCovered, totalStmts int
	result := make([]PackageCoverage, 0, len(pkgs))
	for pkg, acc := range pkgs {
		pct := 0.0
		if acc.total > 0 {
			pct = float64(acc.covered) / float64(acc.total) * 100
		}
		result = append(result, PackageCoverage{
			Path:       pkg,
			Percentage: pct,
			Covered:    acc.covered,
			Total:      acc.total,
		})
		totalCovered += acc.covered
		totalStmts += acc.total
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Path < result[j].Path
	})

	totalPct := 0.0
	if totalStmts > 0 {
		totalPct = float64(totalCovered) / float64(totalStmts) * 100
	}

	return &CoverageReport{
		Total:    totalPct,
		Packages: result,
	}, nil
}

// parseCoverageLine parses a Go coverage profile line.
// Format: file:startLine.startCol,endLine.endCol numStatements count
func parseCoverageLine(line string) (file string, stmts int, count int, err error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", 0, 0, fmt.Errorf("empty line")
	}

	lastSpace := strings.LastIndex(line, " ")
	if lastSpace < 0 {
		return "", 0, 0, fmt.Errorf("invalid format")
	}
	count, err = strconv.Atoi(line[lastSpace+1:])
	if err != nil {
		return "", 0, 0, err
	}
	rest := line[:lastSpace]

	lastSpace = strings.LastIndex(rest, " ")
	if lastSpace < 0 {
		return "", 0, 0, fmt.Errorf("invalid format")
	}
	stmts, err = strconv.Atoi(rest[lastSpace+1:])
	if err != nil {
		return "", 0, 0, err
	}
	rest = rest[:lastSpace]

	colonIdx := strings.LastIndex(rest, ":")
	if colonIdx < 0 {
		return "", 0, 0, fmt.Errorf("invalid format")
	}
	file = rest[:colonIdx]

	return file, stmts, count, nil
}

func coveragePackage(file string) string {
	idx := strings.LastIndex(file, "/")
	if idx < 0 {
		return "."
	}
	return file[:idx]
}
