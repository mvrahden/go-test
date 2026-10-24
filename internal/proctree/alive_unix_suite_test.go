//go:build unix

package proctree_test

import (
	"syscall"

	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AliveUnixTestSuite covers the pids kill(2) reads as process groups.
type AliveUnixTestSuite struct{}

func (s *AliveUnixTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *AliveUnixTestSuite) TestAGroupIsNoProcess(t *gotest.T) {
	for t, tc := range gotest.Each(t, []struct {
		Desc string
		pid  int
	}{
		{"pid 0, this process's group", 0},
		{"pid -1, every process", -1},
		{"the negated id of this process's group", -syscall.Getpgrp()},
	}) {
		t.It("reports no process", func(it *gotest.T) {
			gotest.False(it, proctree.Alive(tc.pid))
		})
	}
}
