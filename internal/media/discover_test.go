package media

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverSkipsAppleDoubleSidecars(t *testing.T) {
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "DJI_0001.MP4")
	sidecarPath := filepath.Join(dir, "._DJI_0001.MP4")
	imagePath := filepath.Join(dir, "photo.jpg")

	for _, path := range []string{videoPath, sidecarPath, imagePath} {
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}

	paths, warnings, err := Discover([]string{dir}, true, false)
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings for implicit sidecar skip, got %#v", warnings)
	}
	if len(paths) != 1 {
		t.Fatalf("expected one discovered video, got %#v", paths)
	}
	if filepath.Base(paths[0]) != filepath.Base(videoPath) {
		t.Fatalf("expected %s, got %s", videoPath, paths[0])
	}
}

func TestDiscoverSkipsExplicitAppleDoubleSidecar(t *testing.T) {
	dir := t.TempDir()
	sidecarPath := filepath.Join(dir, "._DJI_0001.MP4")
	if err := os.WriteFile(sidecarPath, []byte("x"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	paths, warnings, err := Discover([]string{sidecarPath}, true, true)
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	if len(paths) != 0 {
		t.Fatalf("expected sidecar to be skipped, got %#v", paths)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected explicit sidecar warning, got %#v", warnings)
	}
}
