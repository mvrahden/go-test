//go:build !unix

package drill

import (
	"errors"
	"io/fs"
	"os"
)

var errLocked = errors.New("locked")

func tryLock(*os.File) error { return errors.New("the drill requires Linux") }

func ours(fs.FileInfo) bool { return true }

func lockError(_ string, err error) error { return err }
