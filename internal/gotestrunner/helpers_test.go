package gotestrunner_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// --- reference implementations (pre-refactor logic) ---

// buildPlainArgs reproduces the exact arg logic of the old RunSingleSuite.
func buildPlainArgs(target gotestrunner.SuiteTarget) (path string, args []string) { //nolint:gocritic // hugeParam: stable API
	if target.RunFilter != "" {
		args = append(args, "-test.run="+target.RunFilter)
	} else {
		args = append(args, fmt.Sprintf("-test.run=^%s$", regexp.QuoteMeta(target.SuiteName)))
	}
	args = append(args, target.RunFlags...)
	if target.CoverProfile != "" {
		args = append(args, "-test.coverprofile="+target.CoverProfile)
	}
	return target.BinaryPath, args
}

// buildTest2JSONArgs reproduces the exact arg logic of the old RunSingleSuiteTest2JSON.
func buildTest2JSONArgs(target gotestrunner.SuiteTarget) (path string, args []string) { //nolint:gocritic // hugeParam: stable API
	var testArgs []string
	if target.RunFilter != "" {
		testArgs = append(testArgs, "-test.run="+target.RunFilter)
	} else {
		testArgs = append(testArgs, fmt.Sprintf("-test.run=^%s$", regexp.QuoteMeta(target.SuiteName)))
	}
	testArgs = append(testArgs, "-test.v=test2json")
	for _, f := range target.RunFlags {
		if f == "-test.v" || strings.HasPrefix(f, "-test.v=") {
			continue
		}
		testArgs = append(testArgs, f)
	}
	if target.CoverProfile != "" {
		testArgs = append(testArgs, "-test.coverprofile="+target.CoverProfile)
	}
	args = []string{"tool", "test2json", "-p", target.Package, "-t", target.BinaryPath}
	args = append(args, testArgs...)
	return "go", args
}

func capturePackageSummary(pkg string, failed bool, d time.Duration, verbose bool) string {
	r, wr, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = wr
	gotestrunner.WritePackageSummary(pkg, failed, d, verbose)
	wr.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	r.Close()
	return buf.String()
}

var jsonTimestampRe = regexp.MustCompile(`\d+\.\d+s`)

func normalizeJSON(raw string) string {
	var lines []string
	for line := range strings.SplitSeq(strings.TrimRight(raw, "\n"), "\n") {
		if line == "" {
			continue
		}
		var ev map[string]any
		if json.Unmarshal([]byte(line), &ev) != nil {
			lines = append(lines, line)
			continue
		}
		ev["Time"] = "«TIME»"
		if _, ok := ev["Elapsed"]; ok {
			ev["Elapsed"] = "«ELAPSED»"
		}
		if output, ok := ev["Output"].(string); ok {
			ev["Output"] = jsonTimestampRe.ReplaceAllString(output, "«TS»")
		}
		normalized := gotest.Must(json.Marshal(ev))
		lines = append(lines, string(normalized))
	}
	return strings.Join(lines, "\n") + "\n"
}
