package drill

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

// Main runs the drill command line and returns its exit code. It must be
// called on the main goroutine, locked to the main thread.
//
//	[--root <repository>] [--binary-dir <dir>] <mutant>...   run the drill
//	--check-shards <dir>                     check the shards' selection files
//	--worker <job.json>                      judge one mutant (internal)
//	--supervise <seconds> <status> <cmd...>  bound a command (internal)
//
// --binary-dir names the directory holding this executable, removed on exit.
func Main(args []string) int {
	var root string
	for len(args) >= 2 && (args[0] == "--root" || args[0] == "--binary-dir") {
		if args[0] == "--root" {
			root = args[1]
		} else {
			defer func(dir string) { _ = os.RemoveAll(dir) }(args[1])
		}
		args = args[2:]
	}
	code, err := dispatch(root, args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "DRILL  "+err.Error())
		return 1
	}
	return code
}

func dispatch(root string, args []string) (int, error) {
	switch {
	case len(args) == 2 && args[0] == "--worker":
		data, err := os.ReadFile(args[1])
		if err != nil {
			return 0, err
		}
		var spec Spec
		if err := json.Unmarshal(data, &spec); err != nil {
			return 0, err
		}
		Worker(&spec, RunCommand)
		return 0, nil
	case len(args) == 2 && args[0] == "--check-shards":
		found := FindSelections(args[1])
		problems := CheckShards(found)
		for _, problem := range problems {
			fmt.Println("DRILL  " + problem)
		}
		if len(problems) > 0 {
			return 1, nil
		}
		fmt.Printf("drill  %d shards agree on one assignment\n", len(found))
		return 0, nil
	case len(args) >= 4 && args[0] == "--supervise":
		seconds, err := strconv.ParseFloat(args[1], 64)
		if err != nil {
			return 0, err
		}
		return superviseCommand(seconds, args[2], args[3:])
	}
	return runDrill(root, args)
}
