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
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
)

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

func runWatch(inv Invocation) int {
	args := inv.DefaultArgs()
	if inv.Config.Debounce.Duration() > 0 && !hasFlag(args, "--debounce") {
		args = append([]string{"--debounce=" + inv.Config.Debounce.Duration().String()}, args...)
	}
	ownArgs, goTestArgs, err := SplitArgs(args, watchAllowed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	jsonMode, goTestArgs := stripJSONFlag(goTestArgs)
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
	parallel, err := parseParallelFlag(ownArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	if parallel == 0 {
		parallel = inv.Config.Parallel
	}
	patterns := ExtractPackagePatterns(goTestArgs)

	cfg := ExecConfig{
		GoTestArgs:      goTestArgs,
		PackagePatterns: patterns,
		SetupTimeout:    setupTimeout,
		Debug:           slices.Contains(ownArgs, "--debug"),
		CI:              slices.Contains(ownArgs, "--ci"),
		UpdateSnapshots: slices.Contains(ownArgs, "--update-snapshots"),
		Parallel:        parallel,
	}

	ctx, stop := signal.NotifyContext(context.Background(), shutdownSignals...)
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
	classified := gotestrunner.ClassifyGoTestArgs(cfg.GoTestArgs)
	loadFlags := gotestrunner.StripCoverBuildFlags(classified.BuildFlags)
	loaded, err := gotestgen.LoadPackages(cfg.PackagePatterns, loadFlags)
	if err != nil {
		if jsonMode {
			fmt.Printf("{\"Action\":\"watch-error\",\"Output\":%q}\n", err.Error())
		} else {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		}
		return 2
	}

	if cfg.CI {
		if code, err := enforceFocusGuard(loaded); err != nil {
			if jsonMode {
				fmt.Printf("{\"Action\":\"watch-error\",\"Output\":%q}\n", err.Error())
			} else {
				fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			}
			return 2
		} else if code != 0 {
			return code
		}
	}

	overlay, cleanup, err := gotestrunner.GenerateOverlay(loaded, cfg.Debug)
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
		cfg.JSON = true
	}

	mode := gotestrunner.RunBatchText
	if cfg.JSON {
		mode = gotestrunner.RunStreamJSON
	}

	result, err := gotestrunner.RunPipeline(ctx, gotestrunner.PipelineConfig{
		GoTestArgs:      cfg.GoTestArgs,
		SetupTimeout:    cfg.SetupTimeout,
		UpdateSnapshots: cfg.UpdateSnapshots,
		Parallel:        cfg.Parallel,
		Streaming:       false,
		OutputMode:      mode,
	}, overlay)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	return result.ExitCode
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
		if gotestrunner.LooksLikePackagePattern(arg) {
			continue
		}
		args = append(args, arg)
	}
	args = append(args, newPatterns...)
	return args
}
