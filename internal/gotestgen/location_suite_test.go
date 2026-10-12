package gotestgen_test

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// LocationTestSuite compiles generated harnesses and checks the file:line a
// failing assertion is reported at.
type LocationTestSuite struct{}

func (s *LocationTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	cfg.Timeout = 3 * time.Minute
	return cfg
}

func (s *LocationTestSuite) TestInlinedHook(t *gotest.T) {
	t.When("a generic hook is inlined into the generated wrapper", func(w *gotest.T) {
		run := runGeneratedSuite(w, "TestLocation_InlinedGenericHook")

		w.It("reports the assertion in the hook, not in the harness", func(it *gotest.T) {
			// test.go:20 is the hook's gotest.Fail in the fixture source.
			for _, suite := range []string{"TestValueTestSuite", "TestPointerTestSuite"} {
				gotest.Regexp(it, `(?m)^=== RUN   `+suite+`/TestOne\n\s+test\.go:20: Fail$`, run.output, suite)
			}
			gotest.NotContains(it, run.output, "gotest_psuite_test.go:", run.output)
		})
	})
}

func (s *LocationTestSuite) TestTestingTypes(t *gotest.T) {
	src := string(gotest.Must(os.ReadFile("testdata/sources/TestLocation_Shapes/test.go")))
	lines := map[string]string{}
	for _, marker := range []string{"std", "std-helper", "std-helper-call", "skip", "fuzz", "on-f", "bench", "bench-helper", "bench-helper-call"} {
		lines[marker] = lineOf(t, src, marker)
	}
	at := func(marker string) string { return lines[marker] }
	tests := runGeneratedSuiteArgs(t, "TestLocation_Shapes", childTimeout)
	benches := runGeneratedSuiteArgs(t, "TestLocation_Shapes", childTimeout, benchOnly...)

	t.It("reports a *testing.T failure at the user's line", func(it *gotest.T) {
		gotest.Regexp(it, `\n\s+`+at("std")+`: Fail\n`, tests.output)
	})
	t.It("reports a *testing.T failure in a helper at the helper's line, behind the method's", func(it *gotest.T) {
		gotest.Regexp(it, `\n\s+`+at("std-helper")+`: `+at("std-helper-call")+`: Fail\n`, tests.output)
	})
	t.It("reports a skip at the user's line", func(it *gotest.T) {
		gotest.Regexp(it, `\n\s+`+at("skip")+`: skipped\n`, tests.output)
	})
	t.It("reports a fuzz body's failure at its line and names no reflect frame", func(it *gotest.T) {
		gotest.Regexp(it, `\n\s+`+at("fuzz")+`: Fail\n`, tests.output)
	})
	t.It("reports a failure on *gotest.F at the user's line", func(it *gotest.T) {
		gotest.Regexp(it, `\n\s+`+at("on-f")+`: Fail\n`, tests.output)
	})
	t.It("reports a benchmark's failure at the user's line", func(it *gotest.T) {
		gotest.Regexp(it, `\n\s+`+at("bench")+`: Fail\n`, benches.output)
	})
	t.It("reports a benchmark helper's failure at the helper's line, behind the method's", func(it *gotest.T) {
		gotest.Regexp(it, `\n\s+`+at("bench-helper")+`: `+at("bench-helper-call")+`: Fail\n`, benches.output)
	})
	t.It("names no file inside gotest, reflect or the runtime", func(it *gotest.T) {
		inside := regexp.MustCompile(`(?m)(^\s+|: )(assertions|b|f|t|value|asm_amd64)\.go:\d+: `)
		gotest.Empty(it, inside.FindAllString(tests.output, -1), tests.output)
		gotest.Empty(it, inside.FindAllString(benches.output, -1), benches.output)
	})
}

func (s *LocationTestSuite) TestFullPath(t *gotest.T) {
	src := string(gotest.Must(os.ReadFile("testdata/sources/TestLocation_Shapes/test.go")))
	helper, call := lineOf(t, src, "std-helper"), lineOf(t, src, "std-helper-call")
	run := runGeneratedSuiteArgs(t, "TestLocation_Shapes", childTimeout, "-test.fullpath", "-test.run=^TestShapesTestSuite$")

	t.It("names the helper's line and the method's by their full paths", func(it *gotest.T) {
		gotest.Regexp(it, `\n\s+\S*[/\\]`+helper+`: \S*[/\\]`+call+`: Fail\n`, run.output)
	})
}

// lineOf names the line of src that carries "// LINE:marker", as go test
// prints it for a fixture's test.go; a Windows checkout ends lines in CRLF.
func lineOf(t *gotest.T, src, marker string) string {
	for i, line := range strings.Split(src, "\n") {
		if strings.HasSuffix(strings.TrimSuffix(line, "\r"), "// LINE:"+marker) {
			return "test\\.go:" + strconv.Itoa(i+1)
		}
	}
	gotest.Fail(t, "no line marked "+marker)
	return ""
}
