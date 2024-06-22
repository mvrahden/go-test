package testgen

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/gotestgen"
)

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

	err = findAndDeleteOldGeneratedFile(scanDir)
	if os.IsNotExist(err) {
		return Args{}, nil, nil, fmt.Errorf("failed generating code. err: no such directory %q", scanDir)
	}
	if err != nil {
		return Args{}, nil, nil, fmt.Errorf("failed inspecting directory %q. err: %w", scanDir, err)
	}

	pkgName, ptestSrcs, pxtestSrcs, err := gotestgen.Generate(scanDir)
	if err != nil {
		return Args{}, nil, nil, fmt.Errorf("failed generating code. err: %w", err)
	}
	if len(ptestSrcs)+len(pxtestSrcs) == 0 {
		return Args{}, nil, nil, fmt.Errorf("failed generating code: no sources to generate\n")
	}

	return Args{AbsPath: scanDir, Package: pkgName, SkipAutoDelete: DEBUG, NArgs: nargs}, ptestSrcs, pxtestSrcs, nil
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

var findAndDeleteOldGeneratedFile = func(dir string) error {
	files, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	buf := bytes.NewBuffer(nil)
	for _, fse := range files {
		buf.Reset()

		if fse.IsDir() {
			continue
		}
		if !strings.HasSuffix(fse.Name(), ".go") {
			continue
		}
		inspectFile := filepath.Join(dir, fse.Name())
		f, err := os.Open(inspectFile)
		if err != nil {
			return fmt.Errorf("failed opening file %q", fse.Name())
		}
		defer f.Close()
		fi, err := f.Stat()
		if err != nil {
			return fmt.Errorf("failed reading file info %q", fse.Name())
		}
		if fi.Size() < 74 {
			continue // hint: skip if less then size of the gen comment
		}
		_, err = io.CopyN(buf, f, 85)
		if errors.Is(err, io.EOF) {
			continue
		}
		if err != nil {
			return fmt.Errorf("failed reading first %d bytes of file %q", buf.Len(), fse.Name())
		}
		if gotestast.GEN_TESTSUITE_FILE.Match(buf.Bytes()) {
			os.Remove(inspectFile)
		}
	}
	return nil
}
