package gotestgen

import (
	"fmt"
	"sync"

	"golang.org/x/tools/go/packages"
)

type cacheRecord struct {
	pkgs []*packages.Package
	err  error
}

var (
	// ErrNoGoPkg = errors.New("not a Go package")
	// cacheMu is a write-lock for cache.
	cacheMu = new(sync.Mutex)
	// cache keeps holds all packages per load-path.
	cache = make(map[string]cacheRecord)
)

func LoadCached(pkgPath string) ([]*packages.Package, error) {
	cacheMu.Lock()
	res, ok := cache[pkgPath]
	cacheMu.Unlock()
	if ok {
		if res.err != nil {
			return nil, fmt.Errorf("failed loading packages. err: %w", res.err)
		}
		return res.pkgs, nil
	}
	pkgs, err := packages.Load(&packages.Config{
		Mode:  packageEvalMode,
		Tests: true,
	}, pkgPath)
	if len(pkgs) == 0 {
		// add result to cache anyway
		writeToCache(pkgPath, cacheRecord{err: err})
		if err != nil {
			return nil, fmt.Errorf("failed loading packages. err: %w", err)
		}
		return nil, nil
	}
	writeToCache(pkgPath, cacheRecord{pkgs: pkgs})
	return pkgs, nil
}

func writeToCache(pkgPath string, l cacheRecord) {
	cacheMu.Lock()
	cache[pkgPath] = l
	cacheMu.Unlock()
}
