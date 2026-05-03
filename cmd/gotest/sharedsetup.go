package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/mvrahden/go-test/internal/gotestgen"
)

type sharedSetupOutput struct {
	StateFile string `json:"stateFile"`
}

func runSharedSetup(args []string) int {
	var dir string
	for _, arg := range args {
		if v, ok := strings.CutPrefix(arg, "--dir="); ok {
			dir = v
		}
	}
	if dir == "" {
		fmt.Fprintln(os.Stderr, "shared-setup: --dir flag is required")
		return 2
	}

	var fixtures []gotestgen.SharedFixtureInfo
	if err := json.NewDecoder(os.Stdin).Decode(&fixtures); err != nil {
		fmt.Fprintf(os.Stderr, "shared-setup: reading fixtures from stdin: %s\n", err)
		return 2
	}
	if len(fixtures) == 0 {
		fmt.Fprintln(os.Stderr, "shared-setup: no fixtures provided")
		return 2
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	proc, err := startSharedFixtures(context.Background(), dir, fixtures)
	if err != nil {
		fmt.Fprintf(os.Stderr, "shared-setup: %s\n", err)
		return 2
	}

	if err := json.NewEncoder(os.Stdout).Encode(sharedSetupOutput{
		StateFile: proc.StateFile(),
	}); err != nil {
		proc.Teardown()
		fmt.Fprintf(os.Stderr, "shared-setup: writing output: %s\n", err)
		return 2
	}

	<-sigCh
	proc.Teardown()
	return 0
}
