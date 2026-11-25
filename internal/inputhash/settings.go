package inputhash

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/about"
)

// Settings are the build settings a hash covers beside the sources.
type Settings struct {
	// Flags are the run's build flags, as Flags normalizes them.
	Flags []string
	// Env are the go command's settings that change a build, by name.
	Env map[string]string
	// Work is the go.work in use, "" for none.
	Work string
	// CLI is the version of the gotest that generates the harness.
	CLI string
}

// envNames are the go env settings a hash covers; GOFLAGS carries build
// flags, GOWORK is read for its lines and not itself hashed.
var envNames = []string{"GOVERSION", "GOOS", "GOARCH", "CGO_ENABLED", "GOEXPERIMENT", "GOFLAGS", "GOWORK"}

// traceFlags change what the go command prints or how it schedules, not
// what it builds.
var traceFlags = map[string]bool{"-x": true, "-n": true, "-work": true, "-v": true, "-json": true, "-p": true, "-vet": true, "-o": true, "-c": true, "-exec": true, "-pkgdir": true, "-modcacherw": true}

// Flags normalizes build flags for a hash: each flag with its value as one
// word, the flags that change no build left out, sorted.
func Flags(buildFlags []string) []string {
	var out []string
	for i := 0; i < len(buildFlags); i++ {
		f := buildFlags[i]
		name, _, hasEquals := strings.Cut(f, "=")
		if !hasEquals && i+1 < len(buildFlags) && valueFlag(name) {
			f += "=" + buildFlags[i+1]
			i++
		}
		if traceFlags[name] {
			continue
		}
		out = append(out, f)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// valueFlag reports whether a build flag takes a value.
func valueFlag(name string) bool {
	switch name {
	case "-asmflags", "-buildmode", "-compiler", "-covermode", "-coverpkg", "-gccgoflags", "-gcflags",
		"-installsuffix", "-ldflags", "-mod", "-modfile", "-overlay", "-p", "-pgo", "-pkgdir", "-tags",
		"-toolexec", "-exec", "-o", "-vet":
		return true
	}
	return false
}

// ReadSettings reads the go command's settings in dir ("" the current
// directory) for a build under flags.
func ReadSettings(ctx context.Context, dir string, buildFlags []string) (Settings, error) {
	s := Settings{Flags: Flags(buildFlags), Env: map[string]string{}, CLI: about.ResolvedVersion()}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", append([]string{"env", "-json"}, envNames...)...) //nolint:gosec // G204: the go tool with fixed arguments
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return s, fmt.Errorf("go env: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if err := json.Unmarshal(stdout.Bytes(), &s.Env); err != nil {
		return s, fmt.Errorf("go env: %w", err)
	}
	if w := s.Env["GOWORK"]; w != "" && w != "off" {
		s.Work = w
	}
	delete(s.Env, "GOWORK")
	if goflags := s.Env["GOFLAGS"]; goflags != "" {
		s.Env["GOFLAGS"] = strings.Join(Flags(strings.Fields(goflags)), " ")
	}
	return s, nil
}

// lines are the settings as the hash lists them.
func (s *Settings) lines() []string {
	var out []string
	for _, f := range s.Flags {
		out = append(out, "flag "+f)
	}
	for _, name := range envNames {
		if v, ok := s.Env[name]; ok && v != "" {
			out = append(out, "env "+name+"="+v)
		}
	}
	if s.Work != "" {
		for _, l := range modLines(s.Work, true) {
			out = append(out, "gowork "+l)
		}
	}
	return append(out, "cli "+s.CLI)
}
