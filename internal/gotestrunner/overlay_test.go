package gotestrunner //nolint:stdlib-test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mvrahden/go-test/about"
	"github.com/mvrahden/go-test/internal/gotestgen"
)

func TestWriteOverlay(t *testing.T) {
	results := gotestgen.GenerateResults{
		{AbsPath: "/fake/pkg/a", PTest: []byte("package a\n"), PXTest: []byte("package a_test\n")},
		{AbsPath: "/fake/pkg/b", PTest: []byte("package b\n")},
	}

	tmpDir, err := WriteOverlay(results)
	if err != nil {
		t.Fatalf("WriteOverlay: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	data, err := os.ReadFile(filepath.Join(tmpDir, "overlay.json"))
	if err != nil {
		t.Fatalf("read overlay.json: %v", err)
	}
	var ov overlayJSON
	if err := json.Unmarshal(data, &ov); err != nil {
		t.Fatalf("unmarshal overlay.json: %v", err)
	}

	wantKeys := map[string]bool{
		filepath.Join("/fake/pkg/a", about.PSuite):  true,
		filepath.Join("/fake/pkg/a", about.PXSuite): true,
		filepath.Join("/fake/pkg/b", about.PSuite):  true,
	}
	if len(ov.Replace) != len(wantKeys) {
		t.Fatalf("expected %d overlay entries, got %d", len(wantKeys), len(ov.Replace))
	}
	for virtual, real := range ov.Replace {
		if !wantKeys[virtual] {
			t.Errorf("unexpected overlay key: %s", virtual)
		}
		if _, err := os.Stat(real); err != nil {
			t.Errorf("overlay target missing: %s: %v", real, err)
		}
	}

	bPXSuite := filepath.Join("/fake/pkg/b", about.PXSuite)
	if _, ok := ov.Replace[bPXSuite]; ok {
		t.Error("pkg/b should not have PXSuite mapping (empty PXTest)")
	}
}

func TestWriteOverlay_Empty(t *testing.T) {
	tmpDir, err := WriteOverlay(nil)
	if err != nil {
		t.Fatalf("WriteOverlay: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	data, err := os.ReadFile(filepath.Join(tmpDir, "overlay.json"))
	if err != nil {
		t.Fatalf("read overlay.json: %v", err)
	}
	var ov overlayJSON
	if err := json.Unmarshal(data, &ov); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(ov.Replace) != 0 {
		t.Fatalf("expected 0 overlay entries, got %d", len(ov.Replace))
	}
}
