package gotestrunner

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/protocol"
)

type OverlayResult struct {
	// OverlayDir holds the run's overlay.json and the generated files it
	// names, inside WorkDir: no other run shares or removes it.
	OverlayDir string
	WorkDir    string
	// BinaryCacheRoot is the cache root the run's test binaries link under
	// (see binaryCacheDir); empty under --no-cache or without a usable root.
	BinaryCacheRoot string
	OverlayFlag     string
	SharedFixtures  []gotestgen.SharedFixtureInfo
	SuitePackages   []string
	// BrokenPackages are pattern-matched packages that failed to load. The
	// pipeline books each one as a failed package so the run cannot report
	// success while some of its packages never became runnable.
	BrokenPackages  []gotestgen.BrokenPackage
	NoSuitePackages []string
	// groups is the run's --group and --skip-group selection.
	groups groupSelection
	// Declared are the units the packages declare, which the census holds a
	// green run to.
	Declared DeclaredUnits
	// Plan is every static fact the run reads about the packages' suites.
	Plan *RunPlan
}

// plan is the overlay's plan; an overlay built without one has an empty plan.
func (o *OverlayResult) plan() *RunPlan {
	if o.Plan != nil {
		return o.Plan
	}
	return &RunPlan{}
}

func GenerateOverlay(loaded []*gotestgen.LoadResult, broken []gotestgen.BrokenPackage, debug bool, noCache bool, harvestSeeds bool) (*OverlayResult, func(), error) {
	allResults, allSharedFixtures, err := gotestgen.GenerateFromLoadedOpts(loaded, harvestSeeds)
	if err != nil {
		return nil, nil, err
	}

	CleanStaleOverlays()

	binaryCacheRoot := ""
	if !noCache {
		if root, err := cacheRoot(); err == nil {
			binaryCacheRoot = root
		}
	}

	workDir, err := os.MkdirTemp(workDirParent(), "gotest-work-*")
	if err == nil {
		// The sweep of a later run removes the work dir of a run that died.
		err = writePIDFile(workDir)
	}
	if err != nil {
		if workDir != "" {
			os.RemoveAll(workDir)
		}
		return nil, nil, fmt.Errorf("create work dir: %w", err)
	}
	overlayDir := filepath.Join(workDir, "overlay")
	if err := writeOverlayFiles(overlayDir, allResults); err != nil {
		os.RemoveAll(workDir)
		return nil, nil, err
	}

	cleanup := func() { os.RemoveAll(workDir) }
	if debug {
		// Kept for inspection: the sweep leaves a marked dir alone.
		_ = markKept(workDir)
		fmt.Fprintf(os.Stderr, "DEBUG: overlay dir: %s\nDEBUG: work dir: %s\nDEBUG: binary cache root: %q\n", overlayDir, workDir, binaryCacheRoot)
		cleanup = func() {}
	}

	var suitePkgs []string
	var noSuitePkgs []string
	var declared DeclaredUnits
	for _, r := range allResults {
		declared.Tests = appendCases(declared.Tests, r.PkgPath, r.TestCases)
		declared.Fuzz = appendCases(declared.Fuzz, r.PkgPath, r.FuzzCases)
		declared.Benchmarks = appendCases(declared.Benchmarks, r.PkgPath, r.BenchCases)
		if len(r.PTest) > 0 || len(r.PXTest) > 0 {
			suitePkgs = append(suitePkgs, r.PkgPath)
		} else {
			noSuitePkgs = append(noSuitePkgs, r.PkgPath)
		}
	}

	return &OverlayResult{
		OverlayDir:      overlayDir,
		BinaryCacheRoot: binaryCacheRoot,
		WorkDir:         workDir,
		OverlayFlag:     "-overlay=" + filepath.Join(overlayDir, "overlay.json"),
		SharedFixtures:  allSharedFixtures,
		SuitePackages:   suitePkgs,
		BrokenPackages:  broken,
		NoSuitePackages: noSuitePkgs,
		Declared:        declared,
		Plan:            planRun(allResults),
	}, cleanup, nil
}

type overlayJSON struct {
	Replace map[string]string `json:"Replace"`
}

// workDirParent is the dir a run makes its work dir in: GOTMPDIR when set,
// as the go command does, for a system temp dir mounted noexec; otherwise
// empty, which names the system temp dir.
func workDirParent() string {
	return os.Getenv("GOTMPDIR")
}

// WriteOverlay writes overlay files to a stamped temporary directory of their
// own, for tests that need an overlay without a run.
func WriteOverlay(results gotestgen.GenerateResults) (string, error) {
	return writeToTmpDir(results)
}

func writeToTmpDir(results gotestgen.GenerateResults) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	h := sha256.Sum256([]byte(cwd))
	prefix := fmt.Sprintf("gotest-overlay-%x-", h[:8])
	tmpDir, err := os.MkdirTemp(os.TempDir(), prefix)
	if err != nil {
		return "", fmt.Errorf("create overlay dir: %w", err)
	}

	if err := writePIDFile(tmpDir); err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("write pid file: %w", err)
	}

	if err := writeOverlayFiles(tmpDir, results); err != nil {
		os.RemoveAll(tmpDir)
		return "", err
	}
	return tmpDir, nil
}

// markKept marks dir for the sweep to leave alone.
func markKept(dir string) error {
	return os.WriteFile(filepath.Join(dir, keepFile), nil, 0o600)
}

func writeOverlayFiles(dir string, results gotestgen.GenerateResults) error {
	overlay := overlayJSON{Replace: map[string]string{}}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create overlay dir: %w", err)
	}

	for i, result := range results {
		subDir := filepath.Join(dir, strconv.Itoa(i))
		if err := os.MkdirAll(subDir, 0755); err != nil {
			return fmt.Errorf("create overlay sub dir: %w", err)
		}

		if len(result.PTest) > 0 {
			dst := filepath.Join(subDir, about.PSuite)
			if err := os.WriteFile(dst, result.PTest, 0600); err != nil {
				return fmt.Errorf("write overlay ptest: %w", err)
			}
			overlay.Replace[filepath.Join(result.AbsPath, about.PSuite)] = dst
		}
		if len(result.PXTest) > 0 {
			dst := filepath.Join(subDir, about.PXSuite)
			if err := os.WriteFile(dst, result.PXTest, 0600); err != nil {
				return fmt.Errorf("write overlay pxtest: %w", err)
			}
			overlay.Replace[filepath.Join(result.AbsPath, about.PXSuite)] = dst
		}
		for _, st := range staleStubs(result) {
			dst := filepath.Join(subDir, "stub-"+st.name)
			if err := os.WriteFile(dst, st.src, 0600); err != nil {
				return fmt.Errorf("write overlay stub: %w", err)
			}
			overlay.Replace[filepath.Join(result.AbsPath, st.name)] = dst
		}
	}

	mutants, err := mutantOverlay()
	if err != nil {
		return err
	}
	for name, file := range mutants {
		if _, ok := overlay.Replace[name]; !ok {
			overlay.Replace[name] = file
		}
	}
	data, err := json.Marshal(overlay)
	if err != nil {
		return fmt.Errorf("marshal overlay json: %w", err)
	}
	// Written last: the files it names are all in place by then.
	if err := os.WriteFile(filepath.Join(dir, "overlay.json"), data, 0600); err != nil {
		return fmt.Errorf("write overlay json: %w", err)
	}
	return nil
}

// stub is a generated file on disk and the package clause that stands in
// for it.
type stub struct {
	name string
	src  []byte
}

// staleStubs are the generated files on disk in result's directory that the
// overlay does not regenerate: a run compiles each as its package clause, so
// one left by an earlier generate is never compiled.
func staleStubs(result *gotestgen.GenerateResult) []stub {
	if result.AbsPath == "" {
		return nil
	}
	var stubs []stub
	for _, name := range about.GeneratedFileNames {
		if (name == about.PSuite && len(result.PTest) > 0) || (name == about.PXSuite && len(result.PXTest) > 0) {
			continue
		}
		src, err := os.ReadFile(filepath.Join(result.AbsPath, name))
		if err != nil {
			continue
		}
		if s := about.PackageStub(src); s != nil {
			stubs = append(stubs, stub{name: name, src: s})
		}
	}
	return stubs
}

// cacheRoot returns the gotest cache directory, respecting the GOTEST_CACHE_DIR
// env var with fallback to os.UserCacheDir()/gotest.
func cacheRoot() (string, error) {
	if dir := os.Getenv(protocol.EnvCacheDir); dir != "" {
		return dir, nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "gotest"), nil
}
