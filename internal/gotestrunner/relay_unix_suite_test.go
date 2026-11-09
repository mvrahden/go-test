//go:build unix

package gotestrunner_test

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// RelayGroupTestSuite: on Unix the suite leads a process group of its own
// under the runner, and the converter beside it another, so a signal to the
// suite's group never reaches the converter.
// Sequential: the scenario reads the converter started through one hook.
type RelayGroupTestSuite struct{}

func (s *RelayGroupTestSuite) BeforeEach(t *gotest.T) *converterCtx {
	started := make(chan *os.Process, 4)
	restore := gotestrunner.ExportOnConverterStart(func(p *os.Process) { started <- p })
	return &converterCtx{started: started, restore: restore, hold: t.TempDir()}
}

func (s *RelayGroupTestSuite) AfterEach(t *gotest.T, ctx *converterCtx) { ctx.restore() }

func (s *RelayGroupTestSuite) TestTheSuitesGroupSignalsMissTheConverter(t *gotest.T, ctx *converterCtx) {
	var result gotestrunner.SuiteResult
	wait := gotest.Go(t, func() {
		result = relayed(t.Context(), relayRaw(t, "subtests"), 1, "example.com/relay/subtests", envRelayHold+"="+ctx.hold)
	})
	converter := ctx.converter(t)
	pid, ppid := ctx.halfway(t)
	suiteGroup, err := syscall.Getpgid(pid)
	gotest.NoError(t, err)
	converterGroup, err := syscall.Getpgid(converter.Pid)
	gotest.NoError(t, err)
	gotest.NoError(t, syscall.Kill(-suiteGroup, syscall.SIGINT))
	gotest.NoError(t, syscall.Kill(-suiteGroup, syscall.SIGTERM))
	var caught []byte
	gotest.Eventually(t, 10*time.Second, 10*time.Millisecond, func(poll *gotest.R) {
		caught, err = os.ReadFile(filepath.Join(ctx.hold, "signals"))
		gotest.NoError(poll, err)
		gotest.Len(poll, strings.Split(string(caught), ","), 2)
	})
	running := !errors.Is(converter.Signal(syscall.Signal(0)), os.ErrProcessDone)
	ctx.release(t)
	wait()

	t.When("the suite's process group is sent SIGINT and SIGTERM while the suite is halfway through its stream", func(w *gotest.T) {
		w.It("the suite's parent is the runner", func(it *gotest.T) {
			gotest.Equal(it, os.Getpid(), ppid)
		})
		w.It("the suite leads its group", func(it *gotest.T) {
			gotest.Equal(it, pid, suiteGroup)
		})
		w.It("the converter leads a group of its own", func(it *gotest.T) {
			gotest.Equal(it, converter.Pid, converterGroup)
			gotest.NotEqual(it, syscall.Getpgrp(), converterGroup)
		})
		w.It("reaches the suite with both", func(it *gotest.T) {
			gotest.ElementsMatch(it, []string{"interrupt", "terminated"}, strings.Split(string(caught), ","))
		})
		w.It("leaves the converter running", func(it *gotest.T) {
			gotest.True(it, running)
		})
		w.It("converts the stream once, whole", func(it *gotest.T) {
			gotest.Equal(it, relayGolden(it, "subtests"), eventsOf(it, result.Stdout))
			gotest.Empty(it, result.Stderr)
		})
	})
}

func (s *RelayGroupTestSuite) TestTheConverterStartsApart(t *gotest.T, ctx *converterCtx) {
	cmd := gotestrunner.ExportConverterCmd(t.Context(), "example.com/p", "")
	gotest.NotZero(t, cmd.SysProcAttr)
	t.When("a converter is made", func(w *gotest.T) {
		w.It("starts in a process group of its own", func(it *gotest.T) {
			gotest.True(it, cmd.SysProcAttr.Setpgid)
			gotest.Zero(it, cmd.SysProcAttr.Pgid)
		})
	})
}

// ExecCommandTestSuite: a suite behind an -exec command that ends before the
// suite does keeps its tree, and its stream, until the suite has ended.
type ExecCommandTestSuite struct{}

func (s *ExecCommandTestSuite) TestASuiteOutlivesItsExecCommand(t *gotest.T) {
	hold := t.TempDir()
	wrap := filepath.Join(t.TempDir(), "wrap.sh")
	gotest.NoError(t, os.WriteFile(wrap, []byte("#!/bin/sh\necho $$ > \"$"+envRelayHold+"/wrapper\"\n\"$@\" &\nexit 0\n"), 0o700)) //nolint:gosec // G306: the wrapper must be executable
	target := gotestrunner.SuiteTarget{
		SuiteSpec:  gotestrunner.SuiteSpec{Package: "example.com/relay/subtests", SuiteName: "TestRelayed"},
		BinaryPath: os.Args[0],
		Exec:       []string{wrap},
	}
	env := append(os.Environ(), envRelayRaw+"="+relayRaw(t, "subtests"), envRelayStatus+"=1", envRelayHold+"="+hold)
	var result gotestrunner.SuiteResult
	wait := gotest.Go(t, func() { result = gotestrunner.RunSingleSuite(t.Context(), target, env, true) })
	ctx := &converterCtx{hold: hold}
	ctx.halfway(t)
	wrapper, err := strconv.Atoi(strings.TrimSpace(string(gotest.Must(os.ReadFile(filepath.Join(hold, "wrapper"))))))
	gotest.NoError(t, err)
	gotest.Eventually(t, 10*time.Second, 10*time.Millisecond, func(poll *gotest.R) {
		gotest.ErrorIs(poll, syscall.Kill(wrapper, 0), syscall.ESRCH)
	})
	ctx.release(t)
	wait()

	t.When("the -exec command exits while the suite it started is halfway through its stream", func(w *gotest.T) {
		w.It("keeps the suite running to its end, its stream whole", func(it *gotest.T) {
			gotest.Equal(it, relayGolden(it, "subtests"), eventsOf(it, result.Stdout))
		})
	})
}
