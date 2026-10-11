package main_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// ReplayVerdictTestSuite runs the built CLI's spec/summary --input on
// streams that hold no verdict: none of them may exit 0.
//
//nolint:lifecycle-pair // BeforeAll only wraps the shared binary, which its fixture removes
type ReplayVerdictTestSuite struct {
	CLI *gotestcli.BinarySharedFixture
	cli cliRunner
}

func (s *ReplayVerdictTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.IntegrationSuiteConfig()
}

func (s *ReplayVerdictTestSuite) BeforeAll(t *gotest.T) {
	s.cli = newCLIRunner(s.CLI)
}

// replay feeds stream to the binary on stdin; args follow the subcommand.
func (s *ReplayVerdictTestSuite) replay(t *gotest.T, stream string, args ...string) (stdout, stderr string, code int) {
	cmd := exec.Command(s.cli.binary, args...) //nolint:gosec // G204: controlled binary with fixed args
	cmd.Dir = t.TempDir()
	cmd.Stdin = strings.NewReader(stream)
	var out, errs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errs
	err := cmd.Run()
	var exitErr *exec.ExitError
	gotest.True(t, err == nil || errors.As(err, &exitErr), "running gotest binary: %v\n%s", err, errs.String())
	return out.String(), errs.String(), cmd.ProcessState.ExitCode()
}

const truncatedStream = `{"Action":"run","Package":"example.com/cart","Test":"TestCartTestSuite"}
{"Action":"run","Package":"example.com/cart","Test":"TestCartTestSuite/adds_an_item"}
{"Action":"pass","Package":"example.com/cart","Test":"TestCartTestSuite/adds_an_item","Elapsed":0.01}
{"Action":"pass","Package":"example.com/other","Test":"TestOther","Elapsed":0.01}
{"Action":"pass","Package":"example.com/other","Elapsed":0.02}
`

// A package without test files: a valid stream that ran nothing.
const noTestsStream = `{"Action":"start","Package":"example.com/empty"}
{"Action":"output","Package":"example.com/empty","Output":"?   \texample.com/empty\t[no test files]\n"}
{"Action":"skip","Package":"example.com/empty","Elapsed":0}
`

// A test build that failed, then nothing: the package's verdict never came.
const buildFailCutStream = `{"ImportPath":"example.com/bf [example.com/bf.test]","Action":"build-output","Output":"# example.com/bf [example.com/bf.test]\n"}
{"ImportPath":"example.com/bf [example.com/bf.test]","Action":"build-output","Output":"bf/bf_test.go:3:27: undefined: undefined\n"}
{"ImportPath":"example.com/bf [example.com/bf.test]","Action":"build-fail"}
`

// go build -json, not go test -json: a build failure and no package at all.
const goBuildFailStream = `{"ImportPath":"example.com/bf","Action":"build-output","Output":"# example.com/bf\n"}
{"ImportPath":"example.com/bf","Action":"build-fail"}
`

// The whole go test -json shape of a package that failed to build.
const buildFailStream = buildFailCutStream + `{"Action":"start","Package":"example.com/bf"}
{"Action":"output","Package":"example.com/bf","Output":"FAIL\texample.com/bf [build failed]\n"}
{"Action":"fail","Package":"example.com/bf","Elapsed":0,"FailedBuild":"example.com/bf [example.com/bf.test]"}
`

const wholeRunStream = `{"Action":"start","Package":"example.com/cart"}
{"Action":"run","Package":"example.com/cart","Test":"TestCartTestSuite"}
{"Action":"pass","Package":"example.com/cart","Test":"TestCartTestSuite","Elapsed":0.01}
{"Action":"pass","Package":"example.com/cart","Elapsed":0.02}
`

// A second run appended to a whole one, cut short after its first test.
const concatCutStream = wholeRunStream + `{"Action":"start","Package":"example.com/cart"}
{"Action":"run","Package":"example.com/cart","Test":"TestCartTestSuite"}
`

func (s *ReplayVerdictTestSuite) TestStreamsWithoutVerdict(t *gotest.T) {
	for _, cmd := range []string{"spec", "summary"} {
		t.When(cmd+" reads an empty stream", func(w *gotest.T) {
			_, stderr, code := s.replay(w, "", cmd, "--input=-")

			w.It("exits 2 and says the input holds no events", func(it *gotest.T) {
				gotest.Equal(it, 2, code, stderr)
				gotest.Equal(it, "FAIL: input holds no test events (expected a go test -json stream)\n", stderr)
			})
		})

		t.When(cmd+" reads a file that is not JSON", func(w *gotest.T) {
			path := filepath.Join(w.TempDir(), "events.json")
			gotest.NoError(w, os.WriteFile(path, []byte("PASS\nok  \texample.com/cart\t0.01s\n"), 0o600))
			_, stderr, code := s.replay(w, "", cmd, "--input", path)

			w.It("exits 2 and says the input holds no events", func(it *gotest.T) {
				gotest.Equal(it, 2, code, stderr)
				gotest.Contains(it, stderr, "FAIL: input holds no test events")
			})
		})

		t.When(cmd+" reads a stream cut short", func(w *gotest.T) {
			stdout, stderr, code := s.replay(w, truncatedStream, cmd, "--input=-", "--no-color")

			w.It("exits 2", func(it *gotest.T) {
				gotest.Equal(it, 2, code, stderr)
			})
			w.It("names only the package without a verdict", func(it *gotest.T) {
				gotest.Equal(it, "FAIL: input ends before these packages reported a verdict (stream cut short?):\n  example.com/cart\n", stderr)
			})
			w.It("still renders what the stream holds", func(it *gotest.T) {
				gotest.NotEmpty(it, stdout)
			})
		})

		t.When(cmd+" renders a stream cut short with --render-only", func(w *gotest.T) {
			_, stderr, code := s.replay(w, truncatedStream, cmd, "--input=-", "--render-only")

			w.It("exits 0 without complaint", func(it *gotest.T) {
				gotest.Equal(it, 0, code, stderr)
				gotest.Empty(it, stderr)
			})
		})

		t.When(cmd+" reads a whole stream that ran no tests", func(w *gotest.T) {
			_, stderr, code := s.replay(w, noTestsStream, cmd, "--input=-")

			w.It("exits 0 without complaint", func(it *gotest.T) {
				gotest.Equal(it, 0, code, stderr)
				gotest.Empty(it, stderr)
			})
		})
		t.When(cmd+" reads a stream cut short after a build failed", func(w *gotest.T) {
			_, stderr, code := s.replay(w, buildFailCutStream, cmd, "--input=-")

			w.It("exits 2 naming the package the build was for", func(it *gotest.T) {
				gotest.Equal(it, 2, code, stderr)
				gotest.Equal(it, "FAIL: input ends before these packages reported a verdict (stream cut short?):\n  example.com/bf\n", stderr)
			})
		})

		t.When(cmd+" reads a go build -json stream", func(w *gotest.T) {
			_, stderr, code := s.replay(w, goBuildFailStream, cmd, "--input=-")

			w.It("exits 2: no package reported a verdict", func(it *gotest.T) {
				gotest.Equal(it, 2, code, stderr)
				gotest.Equal(it, "FAIL: input ends before these packages reported a verdict (stream cut short?):\n  example.com/bf\n", stderr)
			})
		})

		t.When(cmd+" reads a whole stream with a package that failed to build", func(w *gotest.T) {
			stdout, stderr, code := s.replay(w, buildFailStream, cmd, "--input=-", "--no-color")

			w.It("exits 2, as the live run does", func(it *gotest.T) {
				gotest.Equal(it, 2, code, stderr)
			})
			w.It("leaves the diagnostics to the rendered output", func(it *gotest.T) {
				gotest.Empty(it, stderr)
				gotest.Contains(it, stdout, "undefined: undefined")
			})
		})

		t.When(cmd+" reads a second run appended to a whole one and cut short", func(w *gotest.T) {
			_, stderr, code := s.replay(w, concatCutStream, cmd, "--input=-")

			w.It("exits 2: the second run reopened the package", func(it *gotest.T) {
				gotest.Equal(it, 2, code, stderr)
				gotest.Equal(it, "FAIL: input ends before these packages reported a verdict (stream cut short?):\n  example.com/cart\n", stderr)
			})
		})

		t.When(cmd+" reads two whole runs appended", func(w *gotest.T) {
			_, stderr, code := s.replay(w, wholeRunStream+wholeRunStream, cmd, "--input=-")

			w.It("exits 0 without complaint", func(it *gotest.T) {
				gotest.Equal(it, 0, code, stderr)
				gotest.Empty(it, stderr)
			})
		})
	}
}
