package gotestrunner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/mvrahden/go-test/about"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/protocol"
)

type OverlayResult struct {
	CacheDir                       string
	WorkDir                        string
	OverlayFlag                    string
	SharedFixtures                 []gotestgen.SharedFixtureInfo
	SuitePackages                  []string
	NoSuitePackages                []string
	SuitesByPkg                    map[string][]string
	DirsByPkg                      map[string]string
	SkippedSuitesByPkg             map[string][]string
	FixtureDepSuites               map[string]map[string]bool
	SuiteRequiredSharedFixtureKeys map[string]map[string][]string
}

func GenerateOverlay(loaded []*gotestgen.LoadResult, debug bool, noCache bool) (*OverlayResult, func(), error) {
	allResults, allSharedFixtures, err := gotestgen.GenerateFromLoaded(loaded)
	if err != nil {
		return nil, nil, err
	}

	CleanStaleOverlays()

	cacheDir, cached, err := writeOverlayCached(allResults, noCache)
	if err != nil {
		return nil, nil, err
	}

	workDir, err := os.MkdirTemp("", "gotest-work-*")
	if err != nil {
		if !cached {
			os.RemoveAll(cacheDir)
		}
		return nil, nil, fmt.Errorf("create work dir: %w", err)
	}

	cleanup := func() {
		os.RemoveAll(workDir)
		if !cached {
			os.RemoveAll(cacheDir)
		}
	}
	if debug {
		fmt.Fprintf(os.Stderr, "DEBUG: overlay dir: %s (cached=%v)\nDEBUG: work dir: %s\n", cacheDir, cached, workDir)
		cleanup = func() {}
	}

	var suitePkgs []string
	var noSuitePkgs []string
	suitesByPkg := map[string][]string{}
	dirsByPkg := map[string]string{}
	skippedSuitesByPkg := map[string][]string{}
	fixtureDepSuites := map[string]map[string]bool{}
	suiteReqKeys := map[string]map[string][]string{}
	for _, r := range allResults {
		if len(r.PTest) > 0 || len(r.PXTest) > 0 {
			suitePkgs = append(suitePkgs, r.PkgPath)
		} else {
			noSuitePkgs = append(noSuitePkgs, r.PkgPath)
		}
		if len(r.SuiteNames) > 0 {
			suitesByPkg[r.PkgPath] = r.SuiteNames
		}
		if r.AbsPath != "" {
			dirsByPkg[r.PkgPath] = r.AbsPath
		}
		if len(r.SkippedSuiteNames) > 0 {
			skippedSuitesByPkg[r.PkgPath] = r.SkippedSuiteNames
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
		CacheDir:                       cacheDir,
		WorkDir:                        workDir,
		OverlayFlag:                    "-overlay=" + filepath.Join(cacheDir, "overlay.json"),
		SharedFixtures:                 allSharedFixtures,
		SuitePackages:                  suitePkgs,
		NoSuitePackages:                noSuitePkgs,
		SuitesByPkg:                    suitesByPkg,
		DirsByPkg:                      dirsByPkg,
		SkippedSuitesByPkg:             skippedSuitesByPkg,
		FixtureDepSuites:               fixtureDepSuites,
		SuiteRequiredSharedFixtureKeys: suiteReqKeys,
	}, cleanup, nil
}

type overlayJSON struct {
	Replace map[string]string `json:"Replace"`
}

// writeOverlayCached attempts to write the overlay to a content-addressable
// cache directory. On cache hit it returns the existing directory. On failure
// to resolve or write to the cache, it falls back to a temporary directory.
// The returned bool indicates whether the directory is cached (true) or
// ephemeral (false).
func writeOverlayCached(results gotestgen.GenerateResults, noCache bool) (string, bool, error) {
	if !noCache {
		root, err := cacheRoot()
		if err == nil {
			dir, err := writeToCacheDir(root, results)
			if err == nil {
				return dir, true, nil
			}
		}
	}

	dir, err := writeToTmpDir(results)
	if err != nil {
		return "", false, err
	}
	return dir, false, nil
}

// WriteOverlay writes overlay files to a temporary directory (legacy behaviour).
// Exported for backward compatibility with tests.
func WriteOverlay(results gotestgen.GenerateResults) (string, error) {
	return writeToTmpDir(results)
}

func writeToCacheDir(root string, results gotestgen.GenerateResults) (string, error) {
	hash := overlayContentHash(results)
	dir := filepath.Join(root, "overlays", hash)

	if _, err := os.Stat(filepath.Join(dir, "overlay.json")); err == nil {
		now := time.Now()
		_ = os.Chtimes(dir, now, now)
		return dir, nil
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create cache dir: %w", err)
	}

	if err := writeOverlayFiles(dir, results); err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
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

	if err := os.WriteFile(filepath.Join(tmpDir, ".pid"), []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("write pid file: %w", err)
	}

	if err := writeOverlayFiles(tmpDir, results); err != nil {
		os.RemoveAll(tmpDir)
		return "", err
	}
	return tmpDir, nil
}

func writeOverlayFiles(dir string, results gotestgen.GenerateResults) error {
	overlay := overlayJSON{Replace: map[string]string{}}

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
	// Write overlay.json last — its presence signals a complete, valid entry.
	if err := os.WriteFile(filepath.Join(dir, "overlay.json"), data, 0600); err != nil {
		return fmt.Errorf("write overlay json: %w", err)
	}
	return nil
}

// overlayContentHash computes a deterministic SHA-256 hash over the generated
// overlay content. The hash covers AbsPath and file content for each result,
// sorted by AbsPath for stability.
func overlayContentHash(results gotestgen.GenerateResults) string {
	type entry struct {
		absPath string
		ptest   []byte
		pxtest  []byte
	}
	entries := make([]entry, len(results))
	for i, r := range results {
		entries[i] = entry{absPath: r.AbsPath, ptest: r.PTest, pxtest: r.PXTest}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].absPath < entries[j].absPath
	})

	h := sha256.New()
	for _, e := range entries {
		h.Write([]byte(e.absPath))
		h.Write([]byte{0})
		h.Write(e.ptest)
		h.Write([]byte{0})
		h.Write(e.pxtest)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
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
