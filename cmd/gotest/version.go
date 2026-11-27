package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"slices"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/internal/gotestaudit"
)

// versionDoc is what version --json prints: the CLI, the Go it was built
// with, the subcommands it has and the schema of each document it writes
// that carries one.
type versionDoc struct {
	Version   string         `json:"version"`
	GoVersion string         `json:"goVersion"`
	Platform  string         `json:"platform"`
	Commands  []string       `json:"commands"`
	Schemas   map[string]int `json:"schemas"`
}

// printVersion prints the version line, or with --json one JSON document.
func printVersion(args []string) {
	if !slices.Contains(args, "--json") {
		fmt.Println(about.LongInfo())
		return
	}
	doc := versionDoc{
		Version:   about.ResolvedVersion(),
		GoVersion: runtime.Version(),
		Platform:  runtime.GOOS + "/" + runtime.GOARCH,
		Schemas:   map[string]int{"audit": gotestaudit.Schema},
	}
	for name := range knownSubcommands {
		doc.Commands = append(doc.Commands, name)
	}
	slices.Sort(doc.Commands)
	_ = json.NewEncoder(os.Stdout).Encode(doc)
}
