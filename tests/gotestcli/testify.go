package gotestcli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// TestifyModuleSharedFixture is a module example.com/m that requires
// testify and the gotest of the checkout under test, tidied once per run:
// writing GoMod and GoSum into a directory makes it that module, so a
// migration starts from testify code that compiles.
type TestifyModuleSharedFixture struct {
	GoMod string
	GoSum string
}

// SharedFixtureConfig: a cold module cache downloads testify.
func (f *TestifyModuleSharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
	cfg := gotest.DefaultFixtureConfig()
	cfg.Timeout = 5 * time.Minute
	// go mod tidy downloads testify; a runner's DNS blip is retried, not
	// counted against the suites that read the module.
	cfg.Retries = 2
	cfg.RetryDelay = 10 * time.Second
	return cfg
}

func (f *TestifyModuleSharedFixture) BeforeAll(ctx context.Context) error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "gotest-testify-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	goMod := "module example.com/m\n\ngo 1.26.0\n\nrequire (\n" +
		"\tgithub.com/mvrahden/go-test v0.0.0-00010101000000-000000000000\n" +
		"\tgithub.com/stretchr/testify v1.11.1\n)\n\n" +
		"replace github.com/mvrahden/go-test => " + root + "\n"
	stub := "package m\n\nimport (\n" +
		"\t_ \"github.com/mvrahden/go-test/pkg/gotest\"\n" +
		"\t_ \"github.com/stretchr/testify/assert\"\n" +
		"\t_ \"github.com/stretchr/testify/mock\"\n" +
		"\t_ \"github.com/stretchr/testify/require\"\n" +
		"\t_ \"github.com/stretchr/testify/suite\"\n)\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "stub.go"), []byte(stub), 0o600); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "go", "mod", "tidy")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go mod tidy: %w\n%s", err, out)
	}
	mod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return err
	}
	sum, err := os.ReadFile(filepath.Join(dir, "go.sum"))
	if err != nil {
		return err
	}
	f.GoMod, f.GoSum = string(mod), string(sum)
	return nil
}

// Write makes dir the module, with files (by slash path) in it.
func (f *TestifyModuleSharedFixture) Write(dir string, files map[string]string) error {
	all := map[string]string{"go.mod": f.GoMod, "go.sum": f.GoSum}
	for name, src := range files {
		all[name] = src
	}
	for name, src := range all {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
			return err
		}
	}
	return nil
}
