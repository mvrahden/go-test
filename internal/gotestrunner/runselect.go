package gotestrunner

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestgen"
)

// ShortMode reports whether the run's tests see testing.Short: -short or
// -test.short before -args, -test.short after it; the last one decides.
func ShortMode(goTestArgs []string) bool {
	short, afterArgs := false, false
	for _, w := range goTestArgs {
		if w == "-args" && !afterArgs {
			afterArgs = true
			continue
		}
		name, value, hasValue := strings.Cut(w, "=")
		if name != "-test.short" && (afterArgs || name != "-short") {
			continue
		}
		if !hasValue {
			short = true
			continue
		}
		on, err := strconv.ParseBool(value)
		short = on || err != nil
	}
	return short
}

// SelectsTests reports whether the run's flags select among each
// package's tests or can leave none to run: -run, -skip, -list, -count=0,
// and after -args -test.run, -test.skip, -test.list, -test.bench,
// -test.count=0 or a flag asking the binary for its usage.
func SelectsTests(goTestArgs []string) bool {
	return mayRunNothing(goTestArgs, ParseExecFlags(goTestArgs).RunFlags) || asksUsage(goTestArgs)
}

// GoneReruns counts the alternatives of only, for the packages of pkgs,
// that match nothing declared declares, and names the first few.
func GoneReruns(only map[string]string, pkgs []string, declared DeclaredUnits) (n int, names string) { //nolint:gocritic // hugeParam: once per run
	units := slices.Concat(declared.Tests, declared.Fuzz, declared.Benchmarks)
	var gone []CensusCase
	for _, pkg := range pkgs {
		if only[pkg] == "" {
			continue
		}
		for _, alt := range strings.Split(only[pkg], "|") {
			if slices.ContainsFunc(units, func(d CensusCase) bool { return d.Pkg == pkg && declares(alt, d.Path) }) {
				continue
			}
			levels := strings.Split(alt, "/")
			for i := range levels {
				levels[i] = strings.TrimSuffix(strings.TrimPrefix(levels[i], "^"), "$")
			}
			gone = append(gone, CensusCase{Pkg: pkg, Path: strings.Join(levels, "/")})
		}
	}
	if len(gone) == 0 {
		return 0, ""
	}
	return len(gone), unitNames(gone)
}

// declares reports whether alt selects the unit at path or its suite.
func declares(alt, path string) bool {
	if matchesLevels(alt, path) {
		return true
	}
	suite, ok := suitePath(path)
	return ok && matchesLevels(alt, suite)
}

// refuseGoneReruns reports, on stderr, a --failed run one of whose failures names a test its package no longer declares: rerunning it would run nothing and pass.
func refuseGoneReruns(cfg PipelineConfig, overlay *OverlayResult) bool { //nolint:gocritic // hugeParam: once per run
	if cfg.Only == nil {
		return false
	}
	pkgs := slices.DeleteFunc(slices.Clone(cfg.PackageOrder), func(pkg string) bool {
		return slices.ContainsFunc(overlay.BrokenPackages, func(b gotestgen.BrokenPackage) bool { return b.PkgPath == pkg })
	})
	n, names := GoneReruns(cfg.Only, pkgs, overlay.Declared)
	if n == 0 {
		return false
	}
	fmt.Fprintf(os.Stderr, "FAIL: --failed: %d failure(s) of the last run name no test any more (%s), so there is nothing to rerun for them; run the packages themselves\n", n, names)
	return true
}
