package media

import "testing"

func TestDefaultConfigScansRecursively(t *testing.T) {
	cfg := defaultConfig()
	if !cfg.Recursive {
		t.Fatal("expected recursive scanning to be enabled by default")
	}
}
