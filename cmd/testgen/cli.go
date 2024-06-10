package main

import (
	"fmt"
	"os"

	"github.com/mvrahden/go-test/internal/cmd/testgen"
)

func main() {
	_, data, _, err := testgen.Execute(os.Args[1:])
	if err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(2)
	}
	fmt.Fprint(os.Stdout, string(data))
}
