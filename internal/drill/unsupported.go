//go:build !linux

package drill

import "errors"

var errLinux = errors.New("the drill requires Linux for subreaper process cleanup")

func superviseCommand(float64, string, []string) (int, error) { return 0, errLinux }

func runDrill(string, []string) (int, error) { return 0, errLinux }
