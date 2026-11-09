package gotestrunner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// test2jsonPath names the converter binary, resolved once per run: the go
// front-end costs 60-145ms per launch, the tool itself under 5ms. It is
// empty when the path cannot be resolved.
var test2jsonPath = sync.OnceValue(func() string {
	return resolveTest2JSON(func() ([]byte, error) {
		return exec.Command("go", "tool", "-n", "test2json").Output()
	})
})

func resolveTest2JSON(locate func() ([]byte, error)) string {
	out, err := locate()
	if err != nil {
		return ""
	}
	path := strings.TrimSpace(string(out))
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return ""
	}
	return path
}

// converterArgv is the converter as a filter, reading the suite's output on
// stdin; without a resolved path it goes through the go tool launcher.
func converterArgv(converter, pkg string) []string {
	argv := []string{converter}
	if converter == "" {
		argv = []string{"go", "tool", "test2json"}
	}
	return append(argv, "-p", pkg, "-t")
}

// converterCmd is a converter for pkg, started apart from any suite's tree
// (see converterAttr).
func converterCmd(ctx context.Context, pkg, dir string) *exec.Cmd {
	argv := converterArgv(test2jsonPath(), pkg)
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // G204: the Go toolchain's own converter
	cmd.Dir = dir
	cmd.SysProcAttr = converterAttr()
	// The launcher's own converter child can hold the output open past it.
	cmd.WaitDelay = converterGrace
	return cmd
}

// converterGrace bounds the wait for a converter once its input has ended.
const converterGrace = 5 * time.Second

// reconvertBound bounds a conversion of a whole stream after the suite ended.
const reconvertBound = time.Minute

// converterStarted sees each converter a relay starts; a variable so tests
// can reach one.
var converterStarted = func(*os.Process) {}

// relay turns a suite's output into test2json events beside the suite: the
// suite writes stdout and stderr to the relay as one stream, which it keeps
// and hands on to a converter of its own, so nothing aimed at the suite ever
// reaches the converter. A converter that ends early is replaced once the
// suite has ended, by a fresh one over the whole stream.
type relay struct {
	pkg, suite, dir string
	events, stderr  *bytes.Buffer

	// raw is written only by the suite's output copier, and read once the
	// suite's output is drained.
	raw bytes.Buffer

	cmd      *exec.Cmd // nil when the converter did not start
	stdin    io.WriteCloser
	exited   chan struct{} // closed once the converter has been waited for
	waitErr  error
	startErr error
	writeErr error
	early    bool // the converter exited before its input ended
	timedOut bool // it had not exited converterGrace after its input ended
}

// startRelay starts a converter for target that writes its events to events
// and its own complaints to stderr. A converter that does not start leaves
// the relay keeping the stream alone, which finish converts.
func startRelay(target SuiteTarget, events, stderr *bytes.Buffer) *relay { //nolint:gocritic // hugeParam: as startSuite
	r := &relay{pkg: target.Package, suite: target.SuiteName, dir: target.Dir, events: events, stderr: stderr}
	cmd := converterCmd(context.Background(), r.pkg, r.dir)
	cmd.Stdout, cmd.Stderr = events, stderr
	stdin, err := cmd.StdinPipe()
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		r.startErr = err
		return r
	}
	converterStarted(cmd.Process)
	r.cmd, r.stdin, r.exited = cmd, stdin, make(chan struct{})
	go func() {
		r.waitErr = cmd.Wait()
		close(r.exited)
	}()
	return r
}

// Write keeps p and hands it to the converter. The suite's output is never
// refused: a converter that took no more is handed nothing after.
func (r *relay) Write(p []byte) (int, error) {
	r.raw.Write(p)
	if r.cmd != nil && r.writeErr == nil {
		if _, err := r.stdin.Write(p); err != nil {
			r.writeErr = err
		}
	}
	return len(p), nil
}

// finish ends the relay once the suite has exited with status and its output
// is drained: the converter gets the crash tail and the end of its input, and
// one that ended early, or never started, is replaced by a fresh converter
// over the whole stream, whose events replace the first's. It fails when the
// stream could not be converted at all.
func (r *relay) finish(status int) error {
	tail := crashTail(r.raw.Bytes(), status)
	r.end(tail)
	how := r.endedEarly()
	if how == "" {
		r.raw = bytes.Buffer{}
		return nil
	}
	in := io.MultiReader(bytes.NewReader(r.raw.Bytes()), bytes.NewReader(tail))
	events, stderr, err := convert(r.pkg, r.dir, in)
	if err != nil {
		fmt.Fprintf(r.stderr, "gotest: %s %s: the converter ended early (%s), and converting the suite's output again failed: %v; its output follows\n", r.pkg, r.suite, how, err)
		r.stderr.Write(r.raw.Bytes())
		return err
	}
	r.events.Reset()
	r.events.Write(events)
	r.stderr.Write(stderr)
	fmt.Fprintf(r.stderr, "gotest: %s %s: the converter ended early (%s); the suite's output was converted again\n", r.pkg, r.suite, how)
	r.raw = bytes.Buffer{}
	return nil
}

// discard ends the converter of a suite that never started.
func (r *relay) discard() { r.end(nil) }

// end hands the converter tail and ends its input, then waits for it,
// converterGrace at most before it is killed.
func (r *relay) end(tail []byte) {
	if r.cmd == nil {
		return
	}
	select {
	case <-r.exited:
		r.early = true
	default:
	}
	if len(tail) > 0 && !r.early && r.writeErr == nil {
		if _, err := r.stdin.Write(tail); err != nil {
			r.writeErr = err
		}
	}
	_ = r.stdin.Close()
	timer := time.NewTimer(converterGrace)
	defer timer.Stop()
	select {
	case <-r.exited:
	case <-timer.C:
		r.timedOut = true
		_ = r.cmd.Process.Kill()
		<-r.exited
	}
}

// endedEarly says how the converter ended before it had converted the whole
// stream, empty when it did.
func (r *relay) endedEarly() string {
	switch {
	case r.startErr != nil:
		return "it did not start: " + r.startErr.Error()
	case r.timedOut:
		return fmt.Sprintf("it had not exited %s after its input ended, and was killed", converterGrace)
	case r.waitErr != nil:
		return r.waitErr.Error()
	case r.early:
		return "it exited before the suite's output ended"
	case r.writeErr != nil:
		return "a write to it failed: " + r.writeErr.Error()
	}
	return ""
}

// convert runs a fresh converter over in.
func convert(pkg, dir string, in io.Reader) (events, stderr []byte, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), reconvertBound)
	defer cancel()
	cmd := converterCmd(ctx, pkg, dir)
	var out, errOut bytes.Buffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = in, &out, &errOut
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			err = errors.Join(err, ctx.Err())
		}
		return nil, nil, err
	}
	return out.Bytes(), errOut.Bytes(), nil
}

// markFraming opens each line a test binary frames for the converter under
// -test.v=test2json.
const markFraming = '\x16'

// crashTail ends a stream the suite left without its final result line, as
// a crash or an exit mid-run leaves it: FAIL for a status other than 0, PASS
// for 0, a TestMain that returned without m.Run. A converter reading a stream
// sets its result, and releases the verdicts it holds back, only on that
// line. Empty for a stream that ends with one.
func crashTail(raw []byte, status int) []byte {
	if endsWithResult(raw) {
		return nil
	}
	tail := string(markFraming) + "FAIL\n"
	if status == 0 {
		tail = string(markFraming) + "PASS\n"
	}
	if len(raw) > 0 && raw[len(raw)-1] != '\n' {
		// A cut last line: the tail starts a line of its own.
		tail = "\n" + tail
	}
	return []byte(tail)
}

// endsWithResult reports whether the last framed line of raw is the binary's
// final PASS or FAIL. Only whole lines count: the converter reads a cut one
// as output.
func endsWithResult(raw []byte) bool {
	end := bytes.LastIndexByte(raw, '\n')
	for end >= 0 {
		start := bytes.LastIndexByte(raw[:end], '\n') + 1
		line := raw[start:end]
		end = start - 1
		if len(line) == 0 || line[0] != markFraming {
			continue
		}
		result := strings.TrimSuffix(string(line[1:]), "\r")
		return result == "PASS" || result == "FAIL" || strings.HasPrefix(result, "FAIL\t")
	}
	return false
}
