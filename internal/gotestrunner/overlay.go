package gotestrunner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/mvrahden/go-test/about"
	"github.com/mvrahden/go-test/internal/gotestgen"
)

type overlayJSON struct {
	Replace map[string]string `json:"Replace"`
}

func WriteOverlay(results gotestgen.GenerateResults) (string, error) {
	tmpDir, err := os.MkdirTemp("", "gotest-overlay-*")
	if err != nil {
		return "", fmt.Errorf("create overlay temp dir: %w", err)
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
