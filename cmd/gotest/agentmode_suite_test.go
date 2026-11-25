package main_test

import (
	main "github.com/mvrahden/go-test/cmd/gotest"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AgentModeTestSuite covers what turns agent output on and the command its
// closing line gives for rerunning the failures.
type AgentModeTestSuite struct{}

func (s *AgentModeTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *AgentModeTestSuite) TestWhatTurnsItOn(t *gotest.T) {
	for t, r := range gotest.Each(t, []struct {
		Desc string
		args []string
		env  map[string]string
		on   bool
		why  string
	}{
		{Desc: "nothing", env: map[string]string{}},
		{Desc: "a detected agent", env: map[string]string{"CLAUDECODE": "1"}, on: true, why: "CLAUDECODE=1"},
		{Desc: "GOTEST_AGENT=0 over a detected agent", env: map[string]string{"AI_AGENT": "x", "GOTEST_AGENT": "0"}, why: "GOTEST_AGENT=0"},
		{Desc: "GOTEST_AGENT=1 without one", env: map[string]string{"GOTEST_AGENT": "1"}, on: true, why: "GOTEST_AGENT=1"},
		{Desc: "--agent over GOTEST_AGENT=0", args: []string{"--agent"}, env: map[string]string{"GOTEST_AGENT": "0"}, on: true, why: "--agent"},
		{Desc: "--agent=false over a detected agent", args: []string{"--agent=false"}, env: map[string]string{"CLAUDECODE": "1"}, why: "--agent"},
	}) {
		on, why, err := main.ExportAgentOutput(r.args, r.env)
		gotest.NoError(t, err)
		gotest.Equal(t, r.on, on)
		gotest.Equal(t, r.why, why)
	}
}

func (s *AgentModeTestSuite) TestTheRerunCommand(t *gotest.T) {
	dir := t.TempDir() // no module: the gotest on PATH
	t.It("keeps the run's flags and drops its packages, its -run and --failed", func(it *gotest.T) {
		gotest.Equal(it, "gotest --failed -race --agent -count=2",
			main.ExportRerunCommand(dir, []string{"-race", "./a/...", "--agent", "-run", "TestX", "-count=2", "--failed", "./b"}))
	})
	t.It("quotes a word the shell would split, and keeps the binary's words after -args", func(it *gotest.T) {
		gotest.Equal(it, `gotest --failed '-tags=a b' -args '-flag=it'\''s'`,
			main.ExportRerunCommand(dir, []string{"-tags=a b", "./...", "-args", "-flag=it's"}))
	})
}

func (s *AgentModeTestSuite) TestWhatMakesAStreamLive(t *gotest.T) {
	for t, r := range gotest.Each(t, []struct {
		Desc    string
		args    []string
		json    bool
		env     map[string]string
		on      bool
		refused bool
	}{
		{Desc: "nothing", json: true},
		{Desc: "--live with -json", args: []string{"--live"}, json: true, on: true},
		{Desc: "GOTEST_LIVE=1 with -json", json: true, env: map[string]string{"GOTEST_LIVE": "1"}, on: true},
		{Desc: "--live=false over GOTEST_LIVE=1", args: []string{"--live=false"}, json: true, env: map[string]string{"GOTEST_LIVE": "1"}},
		{Desc: "GOTEST_LIVE=0", json: true, env: map[string]string{"GOTEST_LIVE": "0"}},
		{Desc: "GOTEST_LIVE=1 in a text run", env: map[string]string{"GOTEST_LIVE": "1"}},
		{Desc: "--live in a text run", args: []string{"--live"}, refused: true},
		{Desc: "--live=false in a text run", args: []string{"--live=false"}},
	}) {
		on, err := main.ExportLiveStream(r.args, r.json, r.env)
		gotest.Equal(t, r.refused, err != nil, "err: %v", err)
		gotest.Equal(t, r.on, on)
	}
}
