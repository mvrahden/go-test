package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

type ExecConfig struct {
	MaxConcurrency int
	TestRecursive  bool // wether to walk the directory tree (./...) -> if not set, test single package
	CWD            string
	RawPath        string // raw path argument (e.g.: "./...", "net/http" ...)
}

func ParseArgs(inArgs []string) (_ ExecConfig, nargs []string, _ error) {
	args, nargs, err := splitArgs(inArgs)
	if err != nil {
		return ExecConfig{}, nil, err
	}
	cfg, err := parseArgs(args)
	if err != nil {
		return ExecConfig{}, nil, err
	}
	return cfg, nargs, nil
}

func parseArgs(args []string) (ExecConfig, error) {
	if len(args) == 0 {
		return ExecConfig{}, fmt.Errorf("missing args")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ExecConfig{}, err
	}
	cfg := ExecConfig{
		MaxConcurrency: 8, // TODO: Determine
		CWD:            cwd,
		RawPath:        args[len(args)-1], // always last arg
	}
	cfg.TestRecursive = cfg.RawPath == "./..."

	return cfg, nil
}

func splitArgs(inArgs []string) (args, nargs []string, _ error) {
	args = inArgs

	DEBUG = slices.Contains(args, "-internal.debug")
	if DEBUG {
		args = slices.DeleteFunc(args, func(v string) bool {
			return v == "-internal.debug"
		})
	}

	// determine nargs
	idx := slices.Index(args, "-")
	if idx == -1 {
		idx = slices.Index(args, "--")
	}
	if idx > -1 {
		if idx == 0 {
			return nil, nil, fmt.Errorf("given \"-\", must provide further nargs")
		}

		nargs = args[idx+1:]
		args = args[:idx]
	}

	return args, nargs, nil
}

func getTargetDirs(cfg ExecConfig) []string {
	if cfg.RawPath == "./..." {
		// TODO: perform Dir-Walk
		panic("not yet supported")
	}
	targetDir := cfg.CWD
	if len(cfg.RawPath) == 0 {
		return []string{targetDir}
	}
	if filepath.IsAbs(cfg.RawPath) {
		targetDir = filepath.Clean(cfg.RawPath)
	} else {
		targetDir = filepath.Join(targetDir, cfg.RawPath)
	}
	return []string{targetDir}
}
