package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/mvrahden/go-test/about"
	"github.com/mvrahden/go-test/internal/cmd/testgen"
)

func PerformTests(cfg ExecConfig, scanDirs []string, nargs []string) (code int) {
	type jobRes struct {
		Dir    string
		Stdout []byte
		Code   int
		Error  error
	}
	jobC := make(chan string, 100)
	resC := make(chan jobRes, 100)
	wg := &sync.WaitGroup{}
	wg.Add(cfg.MaxConcurrency)

	workerFunc := func() {
		for j := range jobC {
			out, code, err := TestPackage(j, nargs)
			resC <- jobRes{j, out, code, err}
		}
		wg.Done()
	}
	for range cfg.MaxConcurrency {
		go workerFunc()
	}
	for _, dir := range scanDirs {
		jobC <- dir
	}

	wg2 := sync.WaitGroup{}
	wg2.Add(1)

	var maxCode int
	collectorFunc := func() {
		for r := range resC {
			if r.Error != nil {
				fmt.Fprintf(os.Stdout, "FAIL  in:     %s\n", r.Dir)
				fmt.Fprintf(os.Stdout, "      due to the following error:\n")
				fmt.Fprintf(os.Stdout, " -->  %s\n", r.Error)
				continue
			}
			if r.Code != 0 {
				fmt.Fprintf(os.Stdout, string(r.Stdout))
				if maxCode < r.Code {
					maxCode = r.Code
				}
				continue
			}
			fmt.Fprintf(os.Stdout, string(r.Stdout))
		}
		wg2.Done()
	}
	go collectorFunc()

	// finish jobs
	close(jobC)
	wg.Wait()

	// finish collection
	close(resC)
	wg2.Wait()

	return maxCode
}

func TestPackage(scanDir string, nargs []string) (out []byte, code int, err error) {
	result, err := testgen.GenerateSuites(scanDir)
	if err != nil {
		return nil, 2, fmt.Errorf("failed generating suites: %w", err)
	}

	if len(result.PTest) > 0 {
		testsuiteFile := filepath.Join(result.AbsPath, about.PSuite)
		err := os.WriteFile(testsuiteFile, result.PTest, os.ModePerm)
		if err != nil {
			return nil, 2, fmt.Errorf("failed writing ptest: %w", err)
		}
	}
	if len(result.PXTest) > 0 {
		testsuiteFile := filepath.Join(result.AbsPath, about.PXSuite)
		err := os.WriteFile(testsuiteFile, result.PXTest, os.ModePerm)
		if err != nil {
			return nil, 2, fmt.Errorf("failed writing pxtest: %w", err)
		}
	}

	nargs = append(nargs, result.AbsPath)
	cmd := exec.Command("go", append([]string{"test"}, nargs...)...)
	if len(nargs) > 0 {
		cmd.Args = append(cmd.Args, nargs...)
	}
	out, _ = cmd.CombinedOutput()
	if !DEBUG { // skip to debug
		os.Remove(filepath.Join(result.AbsPath, about.PSuite))
		os.Remove(filepath.Join(result.AbsPath, about.PXSuite))
	}
	return out, cmd.ProcessState.ExitCode(), nil
}
