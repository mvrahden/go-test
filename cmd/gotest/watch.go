package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/mvrahden/go-test/internal/gotestrunner"
)

func runWatch(args []string) int {
	ownArgs, goTestArgs := SplitArgs(args)
	DEBUG = slices.Contains(ownArgs, "-ƒƒ.internal.debug")
	SPEC = slices.Contains(ownArgs, "--spec")
	patterns := ExtractPackagePatterns(goTestArgs)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Initial run
	fmt.Printf("\033[2m  running tests...\033[0m\n")
	watchRunOnce(goTestArgs, patterns)
	fmt.Printf("\n\033[2m  watching for changes...\033[0m\n")

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: creating watcher: %s\n", err)
		return 2
	}
	defer watcher.Close()

	for _, pattern := range patterns {
		addWatchDirs(watcher, pattern)
	}

	debounce := time.NewTimer(0)
	if !debounce.Stop() {
		<-debounce.C
	}
	var changedDirs map[string]bool

	for {
		select {
		case <-ctx.Done():
			return 0

		case event, ok := <-watcher.Events:
			if !ok {
				return 0
			}
			if !isGoFile(event.Name) {
				continue
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
				continue
			}
			if changedDirs == nil {
				changedDirs = map[string]bool{}
			}
			changedDirs[filepath.Dir(event.Name)] = true
			debounce.Reset(200 * time.Millisecond)

		case <-debounce.C:
			clearTerminal()
			pkgPatterns := dirsToPatterns(changedDirs)
			pkgArgs := replacePatterns(goTestArgs, pkgPatterns)
			watchRunOnce(pkgArgs, pkgPatterns)
			changedDirs = nil
			fmt.Printf("\n\033[2m  watching for changes...\033[0m\n")

		case err, ok := <-watcher.Errors:
			if !ok {
				return 0
			}
			fmt.Fprintf(os.Stderr, "watch error: %s\n", err)
		}
	}
}

func watchRunOnce(goTestArgs []string, patterns []string) int {
	var allDirs []string
	for _, pattern := range patterns {
		dirs, _, err := gotestrunner.SuitesGenerateWithCollectorResults(pattern)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			return 2
		}
		allDirs = append(allDirs, dirs...)
	}
	if !DEBUG {
		defer cleanupGeneratedFiles(allDirs)
	}

	if SPEC {
		return runWithSpec(goTestArgs, nil)
	}

	code, err := gotestrunner.StdlibRunTests(goTestArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	return code
}

func addWatchDirs(w *fsnotify.Watcher, pattern string) {
	dir := strings.TrimSuffix(pattern, "/...")
	if dir == "" || dir == "." {
		dir = "."
	}
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "vendor" || name == "testdata" || name == "node_modules" {
				return filepath.SkipDir
			}
			w.Add(path)
		}
		return nil
	})
}

func isGoFile(name string) bool {
	return strings.HasSuffix(name, ".go")
}

func clearTerminal() {
	fmt.Print("\033[2J\033[H")
}

func dirsToPatterns(dirs map[string]bool) []string {
	patterns := make([]string, 0, len(dirs))
	for dir := range dirs {
		patterns = append(patterns, "./"+filepath.ToSlash(dir))
	}
	return patterns
}

func replacePatterns(originalArgs []string, newPatterns []string) []string {
	var args []string
	for _, arg := range originalArgs {
		if looksLikePackagePattern(arg) {
			continue
		}
		args = append(args, arg)
	}
	args = append(args, newPatterns...)
	return args
}
