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
	BrokenPackages       []gotestgen.BrokenPackage
	NoSuitePackages      []string
	StdlibTestsByPkg     map[string]int // stdlib func TestX counts per package — gotest reports but does not run them
	SuitesByPkg          map[string][]string
	ExclusiveSuitesByPkg map[string]map[string]bool
	// GroupsByPkg is, per package, the named group of each suite declaring
	// one (SuiteConfig.ExclusiveGroup); a suite absent is in the default group.
	GroupsByPkg map[string]map[string]string
	// groups is the run's --group and --skip-group selection.
	groups groupSelection
	// TestlessSuitesByPkg marks the suites with no Test function: they run
	// for their fuzz seeds alone.
	TestlessSuitesByPkg map[string]map[string]bool
	BenchesByPkg        map[string][]string
	FuzzFuncsByPkg      map[string]map[string][]string
	// FuzzParamsByFunc is, per package, the corpus shape of every generated
	// fuzz target: the type of each value the engine feeds it. The
	// stale-corpus pre-flight compares recorded corpus entries against it.
	FuzzParamsByFunc               map[string]map[string][]string
	DirsByPkg                      map[string]string
	SkippedSuitesByPkg             map[string][]string
	FixtureDepSuites               map[string]map[string]bool
	SuiteRequiredSharedFixtureKeys map[string]map[string][]string
	// Declared are the units the packages declare, which the census holds a
	// green run to.
	Declared DeclaredUnits
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

	workDir, err := os.MkdirTemp("", "gotest-work-*")
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
	stdlibByPkg := map[string]int{}
	suitesByPkg := map[string][]string{}
	benchesByPkg := map[string][]string{}
	fuzzFuncsByPkg := map[string]map[string][]string{}
	fuzzParamsByPkg := map[string]map[string][]string{}
	dirsByPkg := map[string]string{}
	skippedSuitesByPkg := map[string][]string{}
	exclusiveSuitesByPkg := map[string]map[string]bool{}
	groupsByPkg := map[string]map[string]string{}
	testlessSuitesByPkg := map[string]map[string]bool{}
	fixtureDepSuites := map[string]map[string]bool{}
	suiteReqKeys := map[string]map[string][]string{}
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
		if r.StdlibTestCount > 0 {
			stdlibByPkg[r.PkgPath] = r.StdlibTestCount
		}
		if len(r.SuiteNames) > 0 {
			suitesByPkg[r.PkgPath] = r.SuiteNames
		}
		if len(r.BenchSuiteNames) > 0 {
			benchesByPkg[r.PkgPath] = r.BenchSuiteNames
		}
		if len(r.FuzzFuncsBySuite) > 0 {
			fuzzFuncsByPkg[r.PkgPath] = r.FuzzFuncsBySuite
		}
		if len(r.FuzzParamsByFunc) > 0 {
			fuzzParamsByPkg[r.PkgPath] = r.FuzzParamsByFunc
		}
		if r.AbsPath != "" {
			dirsByPkg[r.PkgPath] = r.AbsPath
		}
		if len(r.SkippedSuiteNames) > 0 {
			skippedSuitesByPkg[r.PkgPath] = r.SkippedSuiteNames
		}
		if len(r.ExclusiveSuiteNames) > 0 {
			ex := make(map[string]bool, len(r.ExclusiveSuiteNames))
			for _, name := range r.ExclusiveSuiteNames {
				ex[name] = true
			}
			exclusiveSuitesByPkg[r.PkgPath] = ex
		}
		if len(r.GroupBySuite) > 0 {
			groupsByPkg[r.PkgPath] = r.GroupBySuite
		}
		if len(r.TestlessSuiteNames) > 0 {
			tl := make(map[string]bool, len(r.TestlessSuiteNames))
			for _, name := range r.TestlessSuiteNames {
				tl[name] = true
			}
			testlessSuitesByPkg[r.PkgPath] = tl
		}
		if len(r.FixtureDepSuites) > 0 {
			s := make(map[string]bool, len(r.FixtureDepSuites))
			for _, fn := range r.FixtureDepSuites {
				s[fn] = true
			}
			fixtureDepSuites[r.PkgPath] = s
		}
		if len(r.SuiteRequiredSharedFixtureKeys) > 0 {
			suiteReqKeys[r.PkgPath] = r.SuiteRequiredSharedFixtureKeys
		}
	}

	return &OverlayResult{
		OverlayDir:                     overlayDir,
		BinaryCacheRoot:                binaryCacheRoot,
		WorkDir:                        workDir,
		OverlayFlag:                    "-overlay=" + filepath.Join(overlayDir, "overlay.json"),
		SharedFixtures:                 allSharedFixtures,
		SuitePackages:                  suitePkgs,
		BrokenPackages:                 broken,
		NoSuitePackages:                noSuitePkgs,
		StdlibTestsByPkg:               stdlibByPkg,
		SuitesByPkg:                    suitesByPkg,
		ExclusiveSuitesByPkg:           exclusiveSuitesByPkg,
		GroupsByPkg:                    groupsByPkg,
		TestlessSuitesByPkg:            testlessSuitesByPkg,
		BenchesByPkg:                   benchesByPkg,
		FuzzFuncsByPkg:                 fuzzFuncsByPkg,
		FuzzParamsByFunc:               fuzzParamsByPkg,
		DirsByPkg:                      dirsByPkg,
		SkippedSuitesByPkg:             skippedSuitesByPkg,
		FixtureDepSuites:               fixtureDepSuites,
		SuiteRequiredSharedFixtureKeys: suiteReqKeys,
		Declared:                       declared,
	}, cleanup, nil
}

type overlayJSON struct {
	Replace map[string]string `json:"Replace"`
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
