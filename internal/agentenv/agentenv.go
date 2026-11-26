// Package agentenv names the coding agent a process runs under, read from
// the environment the agent gives the commands it starts.
package agentenv

import (
	"os"
	"strings"
)

// EnvAgent is the variable an agent names itself in.
const EnvAgent = "AI_AGENT"

// known are the variables a tool sets for the commands it starts, with the
// value that marks it and the name it is given.
var known = []struct{ env, value, name string }{
	{"CLAUDECODE", "1", "claude-code"},
}

// Name is the agent of this process's environment; "" for none.
func Name() string { return Detect(os.Getenv) }

// Cause is the variable that names the agent getenv describes, as
// NAME=value; "" for none.
func Cause(getenv func(string) string) string {
	if v := strings.TrimSpace(getenv(EnvAgent)); v != "" {
		return EnvAgent + "=" + v
	}
	for _, k := range known {
		if strings.TrimSpace(getenv(k.env)) == k.value {
			return k.env + "=" + k.value
		}
	}
	return ""
}

// Detect is the agent getenv describes: AI_AGENT when set, else the name a
// tool's own variable gives; "" for none.
func Detect(getenv func(string) string) string {
	if v := strings.TrimSpace(getenv(EnvAgent)); v != "" {
		return v
	}
	for _, k := range known {
		if strings.TrimSpace(getenv(k.env)) == k.value {
			return k.name
		}
	}
	return ""
}
