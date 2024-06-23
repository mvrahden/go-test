package main

import (
	"fmt"
	"os"
)

// DEBUG is a hook to help debug generated code.
// It is defined by "-internal.debug" arg.
var DEBUG bool

var onErrFail = func(msg string, err error) {
	if err == nil {
		return
	}
	format := "%s: %s"
	if msg == "" {
		format = "%s%s"
	}
	fmt.Fprint(os.Stderr, fmt.Sprintf(format, msg, err))
	os.Exit(2)
}

func main() {
	args := os.Args[1:]

	cfg, err := ParseArgs(args, true)
	if err != nil {
		onErrFail(fmt.Sprintf("failed parsing args: %s", err), err)
	}

	os.Exit(RunTests(cfg))
}
