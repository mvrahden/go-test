package main

import (
	"fmt"
	"os"

	"github.com/mvrahden/go-test/internal/refactor"
)

func runRefactor(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, `usage: gotest refactor <command> [args]

Commands:
  toggle-focus <file> <identifier>   Toggle F_ prefix on a suite or method
                                     identifier: SuiteName or SuiteName.MethodName`)
		return 1
	}

	switch args[0] {
	case "toggle-focus":
		return runToggleFocus(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown refactor command: %s\n", args[0])
		return 1
	}
}

func runToggleFocus(args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: gotest refactor toggle-focus <file> <identifier>")
		return 1
	}

	filePath := args[0]
	identifier := args[1]

	if err := refactor.ToggleFocus(filePath, identifier); err != nil {
		fmt.Fprintf(os.Stderr, "toggle-focus: %v\n", err)
		return 1
	}

	fmt.Printf("Toggled focus: %s in %s\n", identifier, filePath)
	return 0
}
