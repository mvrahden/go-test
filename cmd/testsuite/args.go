package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	xslices "github.com/mvrahden/go-test/internal/x/slices"
)

type SuiteGeneratorFunc func(pkgName string) error
type SuiteRunnerFunc func(args []string) (out []byte, code int, err error)
type SuiteCleanupFunc func(scanDir string)

type PkgNameExec struct {
	Name string // raw path argument (e.g.: "./...", "net/http" ...)
	// Dir             string
	IsRecursiveWalk bool
}

type ExecConfig struct {
	MaxConcurrency  int
	CWD             string
	NArgs           []string // args per pacakge name job (excluding package names)
	PackageNameList []PkgNameExec
	SuitesGenerate  SuiteGeneratorFunc
	SuitesRun       SuiteRunnerFunc
	SuitesCleanup   SuiteCleanupFunc
}

func ParseArgs(inArgs []string, isStdlib bool) (_ ExecConfig, _ error) {
	args, nargs := splitArgs(inArgs)

	err := parseArgs(args)
	if err != nil {
		return ExecConfig{}, err
	}
	cfg, err := parseNArgs(nargs)
	if err != nil {
		return ExecConfig{}, err
	}
	return cfg, nil
}

func splitArgs(in []string) (args, nargs []string) {
	return xslices.SplitBy(in, func(v string, _ int) bool {
		return strings.HasPrefix(v, "-ƒƒ.")
	})
}

func parseArgs(args []string) error {
	DEBUG = slices.Contains(args, "-ƒƒ.internal.debug")
	return nil
}

func parseNArgs(nargs []string) (ExecConfig, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return ExecConfig{}, err
	}

	cfg := ExecConfig{
		MaxConcurrency: 8, // TODO: Determine
		CWD:            cwd,
		SuitesGenerate: gotestrunner.SuitesGenerate,
		SuitesRun:      gotestrunner.StdlibRunTests,
		SuitesCleanup:  gotestrunner.SuitesCleanup,
		NArgs:          nargs,
	}

	// Search for Package Name List entries in nargs, collect and clear them.

	var sawFlagKey bool
	var pkgNames []string
	var pkgNameIndexes []int
	for idx, v := range nargs {
		if sawFlagKey { // current is a flag value
			sawFlagKey = false
			continue
		}
		isInFlagDeclaration := sawFlagKey
		declaresValue := strings.Contains(v, "=")
		sawFlagKey = v[0] == '-' && !declaresValue              // current is flag key and next is a flag value
		if sawFlagKey || isInFlagDeclaration || declaresValue { // current is either a flag key, a flag value or a flag declaring its value
			continue
		}
		// not a flag
		pkgNames = append(pkgNames, v)
		pkgNameIndexes = append(pkgNameIndexes, idx)
	}

	for _, v := range pkgNames {
		isRecursiveWalk := strings.HasSuffix(v, filepath.Base("..."))
		pkgCfg := PkgNameExec{
			Name: v,
			// Dir:             getPkgTargetDir(cfg.CWD, v, isRecursiveWalk),
			IsRecursiveWalk: isRecursiveWalk}
		cfg.PackageNameList = append(cfg.PackageNameList, pkgCfg)
	}

	return cfg, nil
}
