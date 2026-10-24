//go:build !linux

package drill_test

import (
	"errors"
	"os"

	"github.com/mvrahden/go-test/internal/drill"
)

func runChild([]string) int { return 2 }

func judgeStoppedAtStart(string) (map[string]drill.Verdict, error) {
	return nil, errors.New("the drill runs on Linux only")
}

func lockFile(*os.File) error { return errors.New("the drill runs on Linux only") }
