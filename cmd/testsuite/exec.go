package main

import (
	"fmt"
	"os"
	"sync"
)

type runJobRes struct {
	Dir    string
	Stdout []byte
	Code   int
	Error  error
}

// RunStdlibTests runs pre-test generation and post-test cleanup in
// a fan-out mode and uses `go test` as is.
func RunStdlibTests(cfg ExecConfig) (code int) {
	resC := make(chan runJobRes, 2*cfg.MaxConcurrency)

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

	executeTestrunSequence := func() {
		fanOutJob(cfg, resC, func(pkgName string) error {
			return cfg.SuitesGenerate(pkgName)
		})

		out, code, err := cfg.SuitesRun(cfg.NArgs)
		resC <- runJobRes{
			Stdout: out,
			Code:   code,
			Error:  err,
		}

		if !DEBUG {
			fanOutJob(cfg, resC, func(pkgName string) error {
				return cfg.SuitesCleanup(pkgName)
			})
		}
	}
	executeTestrunSequence()

	// finish collection
	close(resC)
	wg2.Wait()

	return maxCode
}

func fanOutJob(cfg ExecConfig, resC chan<- runJobRes, fn func(string) error) {
	jobC := make(chan string, 100)
	wg := &sync.WaitGroup{}
	wg.Add(cfg.MaxConcurrency)

	workerFunc := func() {
		for j := range jobC {
			err := fn(j)
			if err != nil {
				resC <- runJobRes{j, nil, 2, err}
				continue
			}
		}
		wg.Done()
	}
	for range cfg.MaxConcurrency {
		go workerFunc()
	}
	for _, pkgName := range cfg.PackageNameList {
		jobC <- pkgName.Name
	}

	// finish jobs
	close(jobC)
	wg.Wait()
}
