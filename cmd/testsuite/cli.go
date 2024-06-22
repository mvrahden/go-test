package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"

	"github.com/mvrahden/go-test/about"
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
	args, ptest, pxtest, err := Execute(os.Args[1:])
	onErrFail("", err)
	if len(ptest) > 0 {
		testsuiteFile := filepath.Join(args.AbsPath, about.PSuite)
		onErrFail("failed writing ptest", os.WriteFile(testsuiteFile, ptest, os.ModePerm))
	}
	if len(pxtest) > 0 {
		testsuiteFile := filepath.Join(args.AbsPath, about.PXSuite)
		onErrFail("failed writing pxtest", os.WriteFile(testsuiteFile, pxtest, os.ModePerm))
	}
	// fmt.Println("executing go test at", args.AbsPath, args.Package, args.NArgs)
	cmd := exec.Command("go", "test", "-count", "1", args.AbsPath)
	if len(args.NArgs) > 0 {
		cmd.Args = append(cmd.Args, args.NArgs...)
	}
	out, _ := cmd.CombinedOutput()
	if !args.SkipAutoDelete {
		os.Remove(filepath.Join(args.AbsPath, about.PSuite))
		os.Remove(filepath.Join(args.AbsPath, about.PXSuite))
	}
	switch cmd.ProcessState.ExitCode() {
	case 0:
		fmt.Fprintln(os.Stdout, string(out))
	default:
		fmt.Fprintln(os.Stderr, string(out))
	}
	os.Exit(cmd.ProcessState.ExitCode())
}

// DEBUG is a hook to help debug generated code
var DEBUG bool

type Args struct {
	AbsPath        string
	Package        string
	SkipAutoDelete bool     // internal feature to skip deletion of test suite file
	NArgs          []string // NArgs are unparsed args
}

func Execute(args []string) (_ Args, ptest, pxtest []byte, _ error) {
	var scanPath string // TODO: parse from args
	nargs, err := parseFlags(args, &scanPath)
	if err != nil {
		return Args{}, nil, nil, fmt.Errorf("failed parsing flags. err: %w", err)
	}

	scanDir := getTargetDir(scanPath)
	result, err := testgen.GenerateSuites(scanDir)
	if err != nil {
		return Args{}, nil, nil, err
	}
	return Args{
		AbsPath:        result.AbsPath,
		Package:        result.Package,
		SkipAutoDelete: DEBUG,
		NArgs:          nargs,
	}, result.PTest, result.PXTest, err
}
func parseFlags(args []string, scanPath *string) ([]string, error) {
	DEBUG = slices.Contains(args, "-internal.debug")
	if DEBUG {
		args = slices.DeleteFunc(args, func(v string) bool {
			return v == "-internal.debug"
		})
	}

	// determine nargs
	var nargs []string
	idx := slices.Index(args, "-")
	if idx == -1 {
		idx = slices.Index(args, "--")
	}
	if idx > -1 {
		if idx == 0 {
			return nil, fmt.Errorf("given \"-\", must provide further nargs")
		}

		nargs = args[idx+1:]
		args = args[:idx]
	}
	*scanPath = "." // default: current dir
	if len(args) > 0 {
		*scanPath = args[len(args)-1]
	}
	return nargs, nil
}

func getTargetDir(scanPath string) string {
	targetDir, _ := os.Getwd() // hint: fallback value
	if len(scanPath) == 0 {
		return targetDir
	}
	if filepath.IsAbs(scanPath) {
		targetDir = filepath.Clean(scanPath)
	} else {
		targetDir = filepath.Join(targetDir, scanPath)
	}
	return targetDir
}
