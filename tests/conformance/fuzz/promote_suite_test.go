package fuzz_test

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FuzzPromoteTestSuite: promote turns only the latest session's crashers, and
// only corpus literals, into seeds. Each case stages its own copy of the
// fzpromote package and its own gotest cache, where the session is recorded.
type FuzzPromoteTestSuite struct{}

func (s *FuzzPromoteTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

const (
	promoteTrim    = "FuzzPromoteTestSuite_FuzzTrim"
	promoteMessage = "FuzzPromoteTestSuite_FuzzMessage"
	promoteSeed    = "FuzzPromoteTestSuite_FuzzOnlySeed"
)

// promoteStage is one staged copy of fzpromote and the cache its runs share.
type promoteStage struct {
	dir   string // the package directory
	cache string
	src   string // the suite file's content as staged
}

func stagePromote(t *gotest.T) promoteStage {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	dir, err := testkit.StagePackage(root, filepath.Join("..", "testdata", "fzpromote"), t.TempDir(), "fzmod")
	gotest.NoError(t, err)
	src, err := os.ReadFile(filepath.Join(dir, "fzpromote_suite_test.go"))
	gotest.NoError(t, err)
	return promoteStage{dir: dir, cache: filepath.Join(t.TempDir(), "cache"), src: string(src)}
}

// plant writes a corpus file for target and returns its path.
func (p promoteStage) plant(t *gotest.T, target, name, body string) string {
	dir := filepath.Join(p.dir, "testdata", "fuzz", target)
	gotest.NoError(t, os.MkdirAll(dir, 0o750))
	path := filepath.Join(dir, name)
	gotest.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

// record makes the planted files the latest session's crashers, as a
// session that wrote them would have recorded them.
func (p promoteStage) record(t *gotest.T, files ...string) {
	var session gotestrunner.FuzzSession
	for _, file := range files {
		c, err := gotestrunner.NewSessionCrasher(p.dir, filepath.Base(filepath.Dir(file)), filepath.Base(file))
		gotest.NoError(t, err)
		session.Crashers = append(session.Crashers, c)
	}
	gotest.NoError(t, gotestrunner.WriteFuzzSession(gotestrunner.FuzzSessionFile(p.cache), session))
}

func (p promoteStage) run(t *gotest.T, args ...string) testkit.Run {
	run, err := testkit.RunCLIWithEnv(t.Context(), p.dir, []string{"GOTEST_CACHE_DIR=" + p.cache}, args...)
	gotest.NoError(t, err)
	return run
}

func (p promoteStage) source(t *gotest.T) string {
	src, err := os.ReadFile(filepath.Join(p.dir, "fzpromote_suite_test.go"))
	gotest.NoError(t, err)
	return string(src)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// promoteRefusal is what promote did with a session holding a valid
// crasher and one whose second line is line.
type promoteRefusal struct {
	run        testkit.Run
	bad, good  string
	src, after string
}

func refuse(t *gotest.T, line string) promoteRefusal {
	p := stagePromote(t)
	good := p.plant(t, promoteTrim, "good", "go test fuzz v1\nstring(\"kept\")\n")
	bad := p.plant(t, promoteTrim, "bad", "go test fuzz v1\n"+line+"\n")
	p.record(t, good, bad)
	run := p.run(t, "fuzz", "promote", ".")
	return promoteRefusal{run: run, bad: bad, good: good, src: p.src, after: p.source(t)}
}

func (s *FuzzPromoteTestSuite) TestNonLiterals(t *gotest.T) {
	t.When("a session's crasher file holds anything but Go's corpus literals", func(w *gotest.T) {
		w.It("promote refuses it with exit 1, naming the file and line", func(it *gotest.T) {
			for sub, c := range gotest.Each(it, []struct {
				Desc string
				line string
			}{
				{Desc: "a call", line: `string(os.Getenv("HOME"))`},
				{Desc: "an identifier", line: `string(home)`},
				{Desc: "an operator", line: `int(1 + 2)`},
				{Desc: "a composite literal", line: `[]byte{1, 2}`},
				{Desc: "stray text", line: `string("a") trailing`},
			}) {
				r := refuse(sub, c.line)
				gotest.Equal(sub, 1, r.run.Exit, "stdout: %s\nstderr: %s", r.run.Stdout, r.run.Stderr)
				gotest.Contains(sub, r.run.Stdout+r.run.Stderr, filepath.Join("testdata", "fuzz", promoteTrim, "bad")+":2:")
			}
		})
		w.It("no crasher of the run is promoted and every crasher file stays", func(it *gotest.T) {
			for sub, c := range gotest.Each(it, []struct {
				Desc string
				line string
			}{
				{Desc: "a call", line: `string(os.Getenv("HOME"))`},
				{Desc: "an identifier", line: `string(home)`},
				{Desc: "an operator", line: `int(1 + 2)`},
				{Desc: "a composite literal", line: `[]byte{1, 2}`},
				{Desc: "stray text", line: `string("a") trailing`},
			}) {
				r := refuse(sub, c.line)
				gotest.Equal(sub, r.src, r.after)
				gotest.True(sub, exists(r.bad), "the refused file is gone")
				gotest.True(sub, exists(r.good), "the valid file is gone")
			}
		})
	})
}

func (s *FuzzPromoteTestSuite) TestFilesOutsideTheSession(t *gotest.T) {
	p := stagePromote(t)
	old := p.plant(t, promoteTrim, "old", "go test fuzz v1\nstring(\"from an earlier session\")\n")
	p.record(t)
	run := p.run(t, "fuzz", "promote", ".")
	t.When("a corpus file was not written by the latest fuzz session", func(w *gotest.T) {
		w.It("promote leaves the file and its target's source untouched", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.True(it, exists(old), "the file is gone")
			gotest.Equal(it, p.src, p.source(it))
		})
	})
}

func (s *FuzzPromoteTestSuite) TestDecodedWithoutEcho(t *gotest.T) {
	p := stagePromote(t)
	// One string leaf for a three-leaf fan: the engine refuses the entry
	// before the target runs, so the re-run echoes nothing.
	raw := p.plant(t, promoteMessage, "raw", "go test fuzz v1\nstring(\"x\")\n")
	p.record(t, raw)
	run := p.run(t, "fuzz", "promote", ".")
	t.When("a decoding target's crasher re-run echoes no decoded value", func(w *gotest.T) {
		w.It("promote refuses it with exit 1 and splices no raw leaf", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, p.src, p.source(it))
			gotest.True(it, exists(raw), "the crasher file is gone")
		})
	})
}

// promotion is what promote did with one planted session crasher, and how
// the package's seeds ran after it.
type promotion struct {
	promote, after testkit.Run
	crasher        string
	src, edited    string
}

func promoteOne(t *gotest.T, target string, values ...string) promotion {
	p := stagePromote(t)
	crasher := p.plant(t, target, "crasher", "go test fuzz v1\n"+strings.Join(values, "\n")+"\n")
	p.record(t, crasher)
	promote := p.run(t, "fuzz", "promote", ".")
	after := p.run(t, ".")
	return promotion{promote: promote, after: after, crasher: crasher, src: p.src, edited: p.source(t)}
}

// edgeCase is a crasher whose values sit at the edge of their Go spelling,
// and the failure its target reports on them.
type edgeCase struct {
	Desc   string
	target string
	values []string
	fails  string
}

// failsAsTheCrasherDid checks the promoted seed made the target fail as the
// crasher did, and that it was promoted at all.
func failsAsTheCrasherDid(t *gotest.T, cases []edgeCase) {
	for sub, c := range gotest.Each(t, cases) {
		r := promoteOne(sub, c.target, c.values...)
		gotest.Equal(sub, 0, r.promote.Exit, "stdout: %s\nstderr: %s", r.promote.Stdout, r.promote.Stderr)
		gotest.False(sub, exists(r.crasher), "the crasher file stays")
		gotest.Equal(sub, 1, r.after.Exit, "the promoted seed passes\nseed: %s\nstdout: %s", r.promote.Stdout, r.after.Stdout)
		gotest.Contains(sub, r.after.Stdout+r.after.Stderr, c.fails)
	}
}

// leaf spells a fixed-width number leaf of a decoding target's corpus file:
// its little-endian bytes, as the codec reads them.
func leaf(width int, bits uint64) string {
	b := binary.LittleEndian.AppendUint64(nil, bits)
	return fmt.Sprintf("[]byte(%q)", b[:width])
}

func (s *FuzzPromoteTestSuite) TestDecodedValues(t *gotest.T) {
	const point, edge = "FuzzPromoteTestSuite_FuzzPoint", "FuzzPromoteTestSuite_FuzzEdge"
	one64, one32 := leaf(8, math.Float64bits(1)), leaf(4, uint64(math.Float32bits(1)))
	t.When("a crasher of a decoding target holds a value at the edge of its Go spelling", func(w *gotest.T) {
		w.It("the promoted seed fails the target as the crasher did", func(it *gotest.T) {
			failsAsTheCrasherDid(it, []edgeCase{
				{Desc: "a negative zero", target: point, values: []string{leaf(8, 1<<63), one32}, fails: "a lossy float"},
				{Desc: "a NaN of other bits", target: point, values: []string{leaf(8, 0x7ff8000000000002), one32}, fails: "a lossy float"},
				{Desc: "a float32 negative zero", target: point, values: []string{one64, leaf(4, 1<<31)}, fails: "a lossy float"},
				{Desc: "a float32 NaN of other bits", target: point, values: []string{one64, leaf(4, 0x7fc00001)}, fails: "a lossy float"},
				{Desc: "a float32 signaling NaN", target: point, values: []string{one64, leaf(4, 0x7f800001)}, fails: "a lossy float"},
				{Desc: "integer limits, a non-character rune and invalid UTF-8", target: edge, values: []string{
					leaf(8, math.MaxUint64), leaf(8, 1<<63), leaf(4, math.MaxUint32), `string("\xff\xfe")`, `[]byte("\x00\xff")`,
				}, fails: "the edge values"},
			})
		})
	})
}

func (s *FuzzPromoteTestSuite) TestDecodedArguments(t *gotest.T) {
	r := promoteOne(t, "FuzzPromoteTestSuite_FuzzPointAt",
		leaf(8, 1<<63), leaf(4, uint64(math.Float32bits(1))), leaf(4, math.MaxUint32), leaf(1, 0xff))
	t.When("a decoding target takes several arguments", func(w *gotest.T) {
		w.It("promote adds its crasher as one seed of all of them", func(it *gotest.T) {
			gotest.Equal(it, 0, r.promote.Exit, "stdout: %s\nstderr: %s", r.promote.Stdout, r.promote.Stderr)
			gotest.Equal(it, strings.Count(r.src, "f.Add(")+1, strings.Count(r.edited, "f.Add("))
			gotest.False(it, exists(r.crasher), "the crasher file stays")
		})
		w.It("the promoted seed fails the target as the crasher did", func(it *gotest.T) {
			gotest.Equal(it, 1, r.after.Exit, "the promoted seed passes\nseed: %s\nstdout: %s", r.promote.Stdout, r.after.Stdout)
			gotest.Contains(it, r.after.Stdout+r.after.Stderr, "a lossy point at the edge")
		})
	})
}

// FuzzSessionTestSuite runs real fuzz sessions: Exclusive, because the
// engine's workers take the machine.
type FuzzSessionTestSuite struct{}

func (s *FuzzSessionTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Exclusive = true
	return cfg
}

func (s *FuzzSessionTestSuite) TestSessionCrasher(t *gotest.T) {
	p := stagePromote(t)
	session := p.run(t, "fuzz", "--target="+promoteSeed, ".")
	gotest.Equal(t, 1, session.Exit, "the session found no crasher\nstdout: %s\nstderr: %s", session.Stdout, session.Stderr)
	corpus := filepath.Join(p.dir, "testdata", "fuzz", promoteSeed)
	promote := p.run(t, "fuzz", "promote", ".")
	t.When("a fuzz session finds a crasher", func(w *gotest.T) {
		w.It("promote adds it as a seed and removes its file", func(it *gotest.T) {
			gotest.Equal(it, 0, promote.Exit, "stdout: %s\nstderr: %s", promote.Stdout, promote.Stderr)
			gotest.Equal(it, strings.Count(p.src, "f.Add(")+1, strings.Count(p.source(it), "f.Add("))
			entries, _ := os.ReadDir(corpus)
			gotest.Empty(it, entries)
		})
	})
}

func (s *FuzzSessionTestSuite) TestAnotherCheckout(t *gotest.T) {
	p := stagePromote(t)
	crasher := p.plant(t, promoteTrim, "crasher", "go test fuzz v1\nstring(\"kept\")\n")
	p.record(t, crasher)
	other := stagePromote(t)
	other.cache = p.cache
	session := other.run(t, "fuzz", "--for=1s", "--target="+promoteTrim, ".")
	gotest.Equal(t, 0, session.Exit, "stdout: %s\nstderr: %s", session.Stdout, session.Stderr)
	promote := p.run(t, "fuzz", "promote", ".")
	t.When("a later session sharing the cache fuzzes another checkout's target", func(w *gotest.T) {
		w.It("promote still takes the crashers of the targets it did not fuzz", func(it *gotest.T) {
			gotest.Equal(it, 0, promote.Exit, "stdout: %s\nstderr: %s", promote.Stdout, promote.Stderr)
			gotest.Equal(it, strings.Count(p.src, "f.Add(")+1, strings.Count(p.source(it), "f.Add("))
			gotest.False(it, exists(crasher), "the crasher file stays")
		})
	})
}

func (s *FuzzSessionTestSuite) TestStaleEntry(t *gotest.T) {
	p := stagePromote(t)
	p.plant(t, promoteMessage, "stale", "go test fuzz v1\nstring(\"x\")\n")
	run := p.run(t, "fuzz", "--for=1s", "--target="+promoteMessage, ".")
	warning := ""
	for _, line := range strings.Split(run.Stderr, "\n") {
		if strings.HasPrefix(line, "fuzz: "+promoteMessage+": ") {
			warning = line
		}
	}
	t.When("a fuzz session meets a corpus entry of an older shape", func(w *gotest.T) {
		w.It("warns, naming the entry, without suggesting promote", func(it *gotest.T) {
			gotest.Contains(it, warning, "testdata/fuzz/"+promoteMessage+"/stale", "stderr: %s", run.Stderr)
			gotest.NotContains(it, warning, "promote")
		})
	})
}
