package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/cmd/testgen"
)

func main() {
	args, data, err := testgen.Execute(os.Args[1:])
	if err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(2)
	}
	testsuiteFile := filepath.Join(args.AbsPath, "ƒƒ_suite_test.go")
	os.WriteFile(testsuiteFile, data, os.ModePerm)
	// fmt.Println("executing go test at", args.AbsPath, args.Package)
	cmd := exec.Command("go", "test", "-count", "1", args.AbsPath)
	out, _ := cmd.CombinedOutput()
	switch cmd.ProcessState.ExitCode() {
	case 0:
		fmt.Fprintln(os.Stdout, string(out))
	default:
		fmt.Fprintln(os.Stderr, string(out))
	}
	os.Remove(testsuiteFile)
	os.Exit(cmd.ProcessState.ExitCode())
}
