package media

import "testing"

func TestDefaultConfigScansRecursively(t *testing.T) {
	cfg := defaultConfig()
	if !cfg.Recursive {
		t.Fatal("expected recursive scanning to be enabled by default")
	}
}

func TestNormalizeAnalysisLanguage(t *testing.T) {
	tests := map[string]string{
		"":        "auto",
		"한국어":     "ko",
		"English": "en",
		"中文":      "zh",
		"日本語":     "ja",
		"unknown": "",
	}
	for input, want := range tests {
		if got := normalizeAnalysisLanguage(input); got != want {
			t.Fatalf("normalizeAnalysisLanguage(%q) = %q, want %q", input, got, want)
		}
	}
}
