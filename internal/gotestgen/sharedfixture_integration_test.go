package gotestgen //nolint:stdlib-test

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// TestSharedFixture_Integration_GeneratedSetupBinary verifies that
// GenerateSharedSetup produces a valid, correctly structured Go program
// with context-aware lifecycle calls and field-level JSON serialization.
func TestSharedFixture_Integration_GeneratedSetupBinary(t *testing.T) {
	fixtures := []SharedFixtureInfo{
		{
			Identifier:     "PostgresFixture",
			PkgPath:        "github.com/example/project/tests/fixtures",
			TransferFields: []string{"DSN"},
		},
		{
			Identifier:     "GCSFixture",
			PkgPath:        "github.com/example/project/tests/gcs",
			TransferFields: []string{"Endpoint", "Bucket"},
		},
	}

	src, err := GenerateSharedSetup(fixtures)
	gotest.NoError(t, err)

	code := string(src)

	fset := token.NewFileSet()
	_, parseErr := parser.ParseFile(fset, "setup.go", code, parser.AllErrors)
	gotest.NoError(t, parseErr, "generated code should be valid Go: %s", code)

	gotest.Contains(t, code, "package main")
	gotest.Contains(t, code, `"context"`)
	gotest.Contains(t, code, `"github.com/example/project/tests/fixtures"`)
	gotest.Contains(t, code, `"github.com/example/project/tests/gcs"`)

	gotest.Contains(t, code, "PostgresFixture{}")
	gotest.Contains(t, code, "GCSFixture{}")

	gotest.Contains(t, code, "ctx := context.Background()")
	gotest.Contains(t, code, "sf0.BeforeAll(ctx)")
	gotest.Contains(t, code, "sf1.BeforeAll(ctx)")

	gotest.Contains(t, code, "syscall.SIGTERM")
	gotest.Contains(t, code, "json.NewEncoder(os.Stdout).Encode(state)")

	gotest.True(t, strings.Contains(code, "sf0.DSN"), "DSN should be serialized")
	gotest.True(t, strings.Contains(code, "sf1.Endpoint"), "Endpoint should be serialized")
	gotest.True(t, strings.Contains(code, "sf1.Bucket"), "Bucket should be serialized")

	gcsAfterAllIdx := strings.LastIndex(code, "sf1.AfterAll(ctx)")
	pgAfterAllIdx := strings.LastIndex(code, "sf0.AfterAll(ctx)")
	gotest.True(t, gcsAfterAllIdx < pgAfterAllIdx,
		"expected reverse teardown order: sf1 (GCSFixture) before sf0 (PostgresFixture)")
}

