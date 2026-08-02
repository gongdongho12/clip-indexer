package media

import "testing"

func TestIsWatchedSourceIncludesEmbeddedHelperSources(t *testing.T) {
	for _, path := range []string{"internal/media/vision.go", "internal/media/apple_vision.swift", "internal/media/web/app.html"} {
		if !isWatchedSource(path) {
			t.Fatalf("expected %s to trigger a dev restart", path)
		}
	}
}
