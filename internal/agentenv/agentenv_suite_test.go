package agentenv_test

import (
	"github.com/mvrahden/go-test/internal/agentenv"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AgentEnvTestSuite covers which agent an environment names.
type AgentEnvTestSuite struct{}

func (s *AgentEnvTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *AgentEnvTestSuite) TestDetect(t *gotest.T) {
	type row struct {
		Desc string
		env  map[string]string
		want string
	}
	for t, r := range gotest.Each(t, []row{
		{Desc: "no agent", env: map[string]string{}, want: ""},
		{Desc: "AI_AGENT names it", env: map[string]string{"AI_AGENT": "my-agent"}, want: "my-agent"},
		{Desc: "AI_AGENT wins over a tool's variable", env: map[string]string{"AI_AGENT": "my-agent", "CLAUDECODE": "1"}, want: "my-agent"},
		{Desc: "a blank AI_AGENT names none", env: map[string]string{"AI_AGENT": "  "}, want: ""},
		{Desc: "Claude Code", env: map[string]string{"CLAUDECODE": "1"}, want: "claude-code"},
		{Desc: "CLAUDECODE of another value", env: map[string]string{"CLAUDECODE": "0"}, want: ""},
	}) {
		got := agentenv.Detect(func(k string) string { return r.env[k] })
		gotest.Equal(t, r.want, got)
	}
}
