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
)

func parseWatchFlags(args []string) (jsonMode bool, remaining []string) {
	for _, arg := range args {
		if arg == "-json" {
			jsonMode = true
		} else {
			remaining = append(remaining, arg)
		}
	}
	return
}

func parseDebounceFlag(args []string) (time.Duration, error) {
	for i, arg := range args {
		var raw string
		if v, ok := strings.CutPrefix(arg, "--debounce="); ok {
			raw = v
		} else if arg == "--debounce" && i+1 < len(args) {
			raw = args[i+1]
		} else {
			continue
		}
		d, err := time.ParseDuration(raw)
		if err != nil {
			return 0, fmt.Errorf("invalid --debounce value %q: %w", raw, err)
		}
		if d <= 0 {
			return 0, fmt.Errorf("invalid --debounce value %q: must be positive", raw)
		}
		return d, nil
	}
	return 200 * time.Millisecond, nil
}

func runWatch(args []string) int {
	jsonMode, args := parseWatchFlags(args)
	ownArgs, goTestArgs := SplitArgs(args)
	setupTimeout, err := parseSetupTimeoutFlag(ownArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	debounceDuration, err := parseDebounceFlag(ownArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	patterns := ExtractPackagePatterns(goTestArgs)

	cfg := ExecConfig{
		GoTestArgs:      goTestArgs,
		PackagePatterns: patterns,
		SetupTimeout:    setupTimeout,
		Debug:           slices.Contains(ownArgs, "--debug"),
		CI:              slices.Contains(ownArgs, "--ci"),
		Spec:            slices.Contains(ownArgs, "--spec"),
		UpdateSnapshots: slices.Contains(ownArgs, "--update-snapshots"),
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if !jsonMode {
		fmt.Printf("\033[2m  running tests...\033[0m\n")
	}
	watchRunOnce(ctx, cfg, jsonMode)
	if !jsonMode {
		fmt.Printf("\n\033[2m  watching for changes...\033[0m\n")
	}

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
			debounce.Reset(debounceDuration)

		case <-debounce.C:
			if !jsonMode {
				clearTerminal()
			}
			pkgPatterns := dirsToPatterns(changedDirs)
			pkgArgs := replacePatterns(goTestArgs, pkgPatterns)
			changedCfg := cfg
			changedCfg.GoTestArgs = pkgArgs
			changedCfg.PackagePatterns = pkgPatterns
			watchRunOnce(ctx, changedCfg, jsonMode)
			changedDirs = nil
			if !jsonMode {
				fmt.Printf("\n\033[2m  watching for changes...\033[0m\n")
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return 0
			}
			fmt.Fprintf(os.Stderr, "watch error: %s\n", err)
		}
	}
}

func watchRunOnce(ctx context.Context, cfg ExecConfig, jsonMode bool) int {
	if cfg.CI {
		violations, err := RunFocusGuard(cfg.PackagePatterns)
		if err != nil {
			if jsonMode {
				fmt.Printf("{\"Action\":\"watch-error\",\"Output\":%q}\n", err.Error())
			} else {
				fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			}
			return 2
		}
		if len(violations) > 0 {
			fmt.Fprintln(os.Stderr, "FAIL: focus prefix detected — remove F_ before merging:")
			for _, v := range violations {
				fmt.Fprintln(os.Stderr, v.String())
			}
			return 1
		}
	}

	overlay, cleanup, err := generateOverlay(cfg.PackagePatterns, cfg.Debug)
	if err != nil {
		if jsonMode {
			fmt.Printf("{\"Action\":\"watch-error\",\"Output\":%q}\n", err.Error())
		} else {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		}
		return 2
	}
	defer cleanup()

	if jsonMode {
		fmt.Printf("{\"Action\":\"watch-start\",\"Package\":%q}\n", strings.Join(cfg.PackagePatterns, ","))
		jsonData, code, err := executeTestsJSON(ctx, cfg, overlay)
		if err != nil {
			fmt.Printf("{\"Action\":\"watch-error\",\"Output\":%q}\n", err.Error())
			return 2
		}
		os.Stdout.Write(jsonData)
		return code
	}

	code, err := executeTests(ctx, cfg, overlay)
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
