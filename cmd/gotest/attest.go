package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/internal/protocol"
)

// formatTestResult is summary's --format for the in-toto test-result
// predicate of a saved stream.
const formatTestResult = "intoto-test-result"

// testResult is the in-toto test-result predicate,
// https://in-toto.io/attestation/test-result/v0.1.
type testResult struct {
	Result        string               `json:"result"`
	Configuration []resourceDescriptor `json:"configuration"`
	URL           string               `json:"url,omitempty"`
	PassedTests   []string             `json:"passedTests"`
	WarnedTests   []string             `json:"warnedTests"`
	FailedTests   []string             `json:"failedTests"`
}

// resourceDescriptor is an in-toto ResourceDescriptor.
type resourceDescriptor struct {
	Name        string         `json:"name"`
	URI         string         `json:"uri"`
	Annotations map[string]any `json:"annotations,omitempty"`
}

// runTestResult prints the test-result predicate of the stream --input
// names.
func runTestResult(own OwnFlags, goTestArgs []string) int { //nolint:gocritic // hugeParam: OwnFlags is passed by value everywhere
	input := own.Value("--input")
	if input == "" {
		fmt.Fprintln(os.Stderr, "FAIL: --format="+formatTestResult+" reads a saved stream; give --input=<file> (a run's --events-file)")
		return 2
	}
	if len(goTestArgs) > 0 {
		fmt.Fprintf(os.Stderr, "FAIL: --format=%s runs nothing and takes no packages or go test flags; %s is one\n", formatTestResult, goTestArgs[0])
		return 2
	}
	var r io.Reader = os.Stdin
	if input != "-" {
		f, err := os.Open(input) //nolint:gosec // G304: the stream the user named
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: opening input file: %s\n", err)
			return 2
		}
		defer f.Close()
		r = f
	}
	events, err := gotestspec.ParseEvents(r)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: parsing test events: %s\n", err)
		return 2
	}
	pred, err := testResultOf(events)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	if err := writeAuditOutput(own.Value("--output"), func(w io.Writer) error {
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		return enc.Encode(pred)
	}); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	return 0
}

// verdictKey is a test of a package, "" for the package itself.
type verdictKey struct{ pkg, test string }

// testResultOf reads the predicate from a stream: each leaf test's last
// verdict, the run's manifest and its exit code.
func testResultOf(events []gotestspec.TestEvent) (*testResult, error) {
	run, ok := gotestspec.RunOf(events)
	if !ok {
		return nil, errors.New("the stream holds no run manifest; attest a stream gotest wrote with --events-file")
	}
	verdicts := map[verdictKey]gotestspec.Action{}
	assertions := map[verdictKey]string{}
	assertsBy := map[verdictKey]bool{}
	parents := map[verdictKey]bool{}
	for i := range events {
		ev := &events[i]
		if ev.Package == protocol.RunPackage && ev.RunLevel {
			continue
		}
		k := verdictKey{ev.Package, ev.Test}
		switch ev.Action {
		case gotestspec.ActionPass, gotestspec.ActionFail, gotestspec.ActionSkip:
			verdicts[k] = ev.Action
		case gotestspec.ActionAttr:
			switch ev.Key {
			case protocol.AttrAssertions:
				assertions[k] = ev.Value
			case protocol.AttrAssertsBy:
				assertsBy[k] = true
			}
		}
		if i := strings.LastIndex(ev.Test, "/"); i >= 0 {
			parents[verdictKey{ev.Package, ev.Test[:i]}] = true
		}
	}
	pred := &testResult{PassedTests: []string{}, WarnedTests: []string{}, FailedTests: []string{}}
	failedIn := map[string]bool{}
	for k, v := range verdicts {
		if k.test == "" || parents[k] && v != gotestspec.ActionFail {
			continue
		}
		name := k.pkg + "." + k.test
		switch {
		case v == gotestspec.ActionFail:
			if !failedBelow(verdicts, k) {
				pred.FailedTests = append(pred.FailedTests, name)
				failedIn[k.pkg] = true
			}
		case v == gotestspec.ActionPass && assertions[k] == "0" && !assertsBy[k]:
			pred.WarnedTests = append(pred.WarnedTests, name)
		case v == gotestspec.ActionPass:
			pred.PassedTests = append(pred.PassedTests, name)
		}
	}
	// A package that failed with no test failing, its build say, is listed
	// by its path.
	for k, v := range verdicts {
		if k.test == "" && v == gotestspec.ActionFail && !failedIn[k.pkg] {
			pred.FailedTests = append(pred.FailedTests, k.pkg)
		}
	}
	slices.Sort(pred.PassedTests)
	slices.Sort(pred.WarnedTests)
	slices.Sort(pred.FailedTests)
	switch {
	case len(pred.FailedTests) > 0 || run.ExitCode == nil || *run.ExitCode != 0:
		pred.Result = "FAILED"
	case len(pred.WarnedTests) > 0:
		pred.Result = "WARNED"
	default:
		pred.Result = "PASSED"
	}
	pred.Configuration = []resourceDescriptor{runConfiguration(&run)}
	pred.URL = actionsRunURL()
	return pred, nil
}

// failedBelow reports whether a test below k failed: the failure is
// listed there.
func failedBelow(verdicts map[verdictKey]gotestspec.Action, k verdictKey) bool {
	for o, v := range verdicts {
		if v == gotestspec.ActionFail && o.pkg == k.pkg && strings.HasPrefix(o.test, k.test+"/") {
			return true
		}
	}
	return false
}

// runConfiguration is the CLI that ran the stream, with the Go and the
// arguments it ran with, as the manifest records them.
func runConfiguration(run *gotestspec.RunInfo) resourceDescriptor {
	m := run.Manifest
	uri := "pkg:golang/" + gotestModule
	if m.Version != "" && !strings.ContainsAny(m.Version, " ()") {
		uri += "@" + m.Version
	}
	notes := map[string]any{"version": m.Version, "args": m.Args}
	if m.GoVersion != "" {
		notes["goVersion"] = m.GoVersion
	}
	if m.Commit != "" {
		notes["commit"] = m.Commit
	}
	if m.Dirty != nil {
		notes["dirty"] = *m.Dirty
	}
	if run.ExitCode != nil {
		notes["exitCode"] = *run.ExitCode
	}
	return resourceDescriptor{Name: "gotest", URI: uri, Annotations: notes}
}

// actionsRunURL is the GitHub Actions run this process is a step of; ""
// outside one.
func actionsRunURL() string {
	server, repo, id := os.Getenv("GITHUB_SERVER_URL"), os.Getenv("GITHUB_REPOSITORY"), os.Getenv("GITHUB_RUN_ID")
	if os.Getenv("GITHUB_ACTIONS") != "true" || server == "" || repo == "" || id == "" {
		return ""
	}
	url := server + "/" + repo + "/actions/runs/" + id
	if attempt := os.Getenv("GITHUB_RUN_ATTEMPT"); attempt != "" {
		url += "/attempts/" + attempt
	}
	return url
}
