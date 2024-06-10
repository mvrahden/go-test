package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/cmd/testgen"
)

func main() {
	onErrFail := func(msg string, err error) {
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
	args, ptest, pxtest, err := testgen.Execute(os.Args[1:])
	onErrFail("", err)
	if len(ptest) > 0 {
		testsuiteFile := filepath.Join(args.AbsPath, "ƒƒ_psuite_test.go")
		onErrFail("failed writing ptest", os.WriteFile(testsuiteFile, ptest, os.ModePerm))
	}
	if len(pxtest) > 0 {
		testsuiteFile := filepath.Join(args.AbsPath, "ƒƒ_pxsuite_test.go")
		onErrFail("failed writing pxtest", os.WriteFile(testsuiteFile, pxtest, os.ModePerm))
	}
	// fmt.Println("executing go test at", args.AbsPath, args.Package)
	cmd := exec.Command("go", "test", "-count", "1", args.AbsPath)
	if len(args.Args) > 0 {
		cmd.Args = append(cmd.Args, args.Args...)
	}
	out, _ := cmd.CombinedOutput()
	if !args.SkipAutoDelete {
		os.Remove(filepath.Join(args.AbsPath, "ƒƒ_psuite_test.go"))
		os.Remove(filepath.Join(args.AbsPath, "ƒƒ_pxsuite_test.go"))
	}
	switch cmd.ProcessState.ExitCode() {
	case 0:
		fmt.Fprintln(os.Stdout, string(out))
	default:
		fmt.Fprintln(os.Stderr, string(out))
	}
	os.Exit(cmd.ProcessState.ExitCode())
}
