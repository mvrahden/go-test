//go:build !unix

package drill

import (
	"errors"
	"os"
)

var errLocked = errors.New("locked")

func tryLock(*os.File) error { return errors.New("the drill requires Linux") }
