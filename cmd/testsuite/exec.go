package main

import (
	"fmt"
	"os"
	"sync"
)

func RunTests(cfg ExecConfig) (code int) {
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
			err := cfg.SuitesGenerate(j)
			if err != nil {
				resC <- jobRes{j, nil, 2, err}
				continue
			}

			out, code, err := cfg.SuitesRun(cfg.NArgs)
			resC <- jobRes{j, out, code, err}

			if DEBUG {
				continue // skip cleanup on debug
			}
			cfg.SuitesCleanup(j)
		}
		wg.Done()
	}
	for range cfg.MaxConcurrency {
		go workerFunc()
	}
	for _, pkgName := range cfg.PackageNameList {
		jobC <- pkgName.Name
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
