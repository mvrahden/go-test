package gotestrunner

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/mvrahden/go-test/about"
	"github.com/mvrahden/go-test/internal/gotestgen"
)

type OverlayResult struct {
	TmpDir           string
	OverlayFlag      string
	SharedFixtures   []gotestgen.SharedFixtureInfo
	SuitePackages    []string
	NoSuitePackages  []string
	SuitesByPkg      map[string][]string
	DirsByPkg        map[string]string
	FixtureDepSuites map[string]map[string]bool
}

func GenerateOverlay(loaded []*gotestgen.LoadResult, debug bool) (*OverlayResult, func(), error) {
	allResults, allSharedFixtures, err := gotestgen.GenerateFromLoaded(loaded)
	if err != nil {
		return nil, nil, err
	}

	CleanStaleOverlays()

	tmpDir, err := WriteOverlay(allResults)
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() { os.RemoveAll(tmpDir) }
	if debug {
		fmt.Fprintf(os.Stderr, "DEBUG: overlay dir: %s\n", tmpDir)
		cleanup = func() {}
	}

	var suitePkgs []string
	var noSuitePkgs []string
	suitesByPkg := map[string][]string{}
	dirsByPkg := map[string]string{}
	fixtureDepSuites := map[string]map[string]bool{}
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
		if len(r.FixtureDepSuites) > 0 {
			s := make(map[string]bool, len(r.FixtureDepSuites))
			for _, fn := range r.FixtureDepSuites {
				s[fn] = true
			}
			fixtureDepSuites[r.PkgPath] = s
		}
	}

	return &OverlayResult{
		TmpDir:           tmpDir,
		OverlayFlag:      "-overlay=" + filepath.Join(tmpDir, "overlay.json"),
		SharedFixtures:   allSharedFixtures,
		SuitePackages:    suitePkgs,
		NoSuitePackages:  noSuitePkgs,
		SuitesByPkg:      suitesByPkg,
		DirsByPkg:        dirsByPkg,
		FixtureDepSuites: fixtureDepSuites,
	}, cleanup, nil
}

type overlayJSON struct {
	Replace map[string]string `json:"Replace"`
}

func WriteOverlay(results gotestgen.GenerateResults) (string, error) {
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

	if err := os.WriteFile(filepath.Join(tmpDir, ".pid"), []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("write pid file: %w", err)
	}

	overlay := overlayJSON{Replace: map[string]string{}}

	for i, result := range results {
		subDir := filepath.Join(tmpDir, strconv.Itoa(i))
		if err := os.Mkdir(subDir, 0755); err != nil {
			os.RemoveAll(tmpDir)
			return "", fmt.Errorf("create overlay sub dir: %w", err)
		}

		if len(result.PTest) > 0 {
			dst := filepath.Join(subDir, about.PSuite)
			if err := os.WriteFile(dst, result.PTest, 0644); err != nil {
				os.RemoveAll(tmpDir)
				return "", fmt.Errorf("write overlay ptest: %w", err)
			}
			overlay.Replace[filepath.Join(result.AbsPath, about.PSuite)] = dst
		}
		if len(result.PXTest) > 0 {
			dst := filepath.Join(subDir, about.PXSuite)
			if err := os.WriteFile(dst, result.PXTest, 0644); err != nil {
				os.RemoveAll(tmpDir)
				return "", fmt.Errorf("write overlay pxtest: %w", err)
			}
			overlay.Replace[filepath.Join(result.AbsPath, about.PXSuite)] = dst
		}
	}

	overlayPath := filepath.Join(tmpDir, "overlay.json")
	data, err := json.Marshal(overlay)
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("marshal overlay json: %w", err)
	}
	if err := os.WriteFile(overlayPath, data, 0644); err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("write overlay json: %w", err)
	}

	return tmpDir, nil
}
