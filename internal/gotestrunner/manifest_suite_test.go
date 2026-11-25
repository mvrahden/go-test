package gotestrunner_test

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// RunManifestTestSuite: a stream its manifest opens starts with the run's
// start and ends with the run's verdict, written once.
// Sequential: EndRun and Exit close the process's one live stream.
type RunManifestTestSuite struct{}

// twoPackages is a stream of a passing and a failing package; the failing
// one passed first, so its last verdict is what counts.
const twoPackages = `{"Action":"run","Package":"example.com/a","Test":"TestS"}
{"Action":"run","Package":"example.com/a","Test":"TestS/TestOne"}
{"Action":"pass","Package":"example.com/a","Test":"TestS/TestOne"}
{"Action":"pass","Package":"example.com/a","Test":"TestS"}
{"Action":"pass","Package":"example.com/a"}
{"Action":"run","Package":"example.com/b","Test":"TestS"}
{"Action":"skip","Package":"example.com/b","Test":"TestS"}
{"Action":"pass","Package":"example.com/b"}
{"Action":"fail","Package":"example.com/b"}
`

var manifest = protocol.Manifest{Version: "v1.2.3", GoVersion: "go1.26.0", Args: []string{"-json", "./..."}}

// lastRunEvent decodes the last line of stream.
func lastRunEvent(t *gotest.T, stream string) map[string]any {
	lines := strings.Split(strings.TrimRight(stream, "\n"), "\n")
	var ev map[string]any
	gotest.NoError(t, json.Unmarshal([]byte(lines[len(lines)-1]), &ev))
	return ev
}

func (s *RunManifestTestSuite) TestTheStreamOpensWithTheManifest(t *gotest.T) {
	var stdout bytes.Buffer
	gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false,
		gotestrunner.WithWriters(&stdout, &bytes.Buffer{}), gotestrunner.WithManifest(&manifest))
	t.When("a collector is given a manifest", func(w *gotest.T) {
		w.It("writes the run's start with it before any other event", func(it *gotest.T) {
			lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
			gotest.Len(it, lines, 1)
			gotest.Regexp(it, `^\{"Time":"[^"]+","Action":"start","Package":"gotest run","RunLevel":true,"Manifest":\{"Version":"v1.2.3","GoVersion":"go1.26.0","Args":\["-json","./..."\]\}\}$`, lines[0])
		})
	})
}

func (s *RunManifestTestSuite) TestEndRunClosesTheStream(t *gotest.T) {
	var stdout bytes.Buffer
	c := gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false,
		gotestrunner.WithWriters(&stdout, &bytes.Buffer{}), gotestrunner.WithManifest(&manifest))
	gotestrunner.WriteEvents(c, []byte(twoPackages))
	gotestrunner.EndRun(1)
	closed := stdout.String()
	gotestrunner.EndRun(0)

	t.When("the run ends", func(w *gotest.T) {
		w.It("writes the run's fail with the exit code and the verdicts by package and by test", func(it *gotest.T) {
			ev := lastRunEvent(it, closed)
			gotest.Equal[any](it, "fail", ev["Action"])
			gotest.Equal[any](it, "gotest run", ev["Package"])
			gotest.Equal[any](it, true, ev["RunLevel"])
			result, err := json.Marshal(ev["Result"])
			gotest.NoError(it, err)
			gotest.JSONEq(it, `{"ExitCode":1,"Packages":{"Passed":1,"Failed":1,"Skipped":0},"Tests":{"Passed":2,"Failed":0,"Skipped":1}}`, string(result))
		})
		w.It("writes it once", func(it *gotest.T) {
			gotest.Equal(it, closed, stdout.String())
		})
	})
}

func (s *RunManifestTestSuite) TestAStreamWithoutAManifest(t *gotest.T) {
	var stdout bytes.Buffer
	c := gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
	gotestrunner.WriteEvents(c, []byte(twoPackages))
	gotestrunner.EndRun(1)
	t.When("a stream has no manifest", func(w *gotest.T) {
		w.It("is neither opened nor closed", func(it *gotest.T) {
			gotest.NotContains(it, stdout.String(), `"gotest run"`)
		})
	})
}

func (s *RunManifestTestSuite) TestAnExitClosesTheStream(t *gotest.T) {
	var stdout bytes.Buffer
	c := gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false,
		gotestrunner.WithWriters(&stdout, &bytes.Buffer{}), gotestrunner.WithManifest(&manifest))
	gotestrunner.WriteEvents(c, []byte(`{"Action":"run","Package":"example.com/p","Test":"TestS"}`+"\n"))
	restore := gotestrunner.ExportObserveExit(func(int) {})
	hurry := make(chan struct{})
	close(hurry)
	gotestrunner.Exit(130, hurry)
	restore()
	gotestrunner.ResumeWrites(gotestrunner.ExportExitGate)

	t.When("gotest exits at a further stop", func(w *gotest.T) {
		w.It("books the run as interrupted, then closes the stream with 130", func(it *gotest.T) {
			gotest.Contains(it, stdout.String(), `{"Action":"fail","Package":"interrupted","RunLevel":true}`+"\n")
			ev := lastRunEvent(it, stdout.String())
			gotest.Equal[any](it, "fail", ev["Action"])
			gotest.Equal[any](it, "gotest run", ev["Package"])
			result, ok := ev["Result"].(map[string]any)
			gotest.True(it, ok, "stream:\n%s", stdout.String())
			gotest.Equal[any](it, float64(130), result["ExitCode"])
		})
	})
}
