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

func TestDefaultConfigPromotesDeepSeekAndKeepsGenericLLMAsFallback(t *testing.T) {
	for _, key := range []string{
		"DEEPSEEK_API_KEY", "DEEPSEEK_BASE_URL", "DEEPSEEK_MODEL",
		"LLM_API_KEY", "LLM_BASE_URL", "LLM_MODEL",
		"OPENAI_API_KEY", "OPENAI_BASE_URL", "OPENAI_MODEL",
		"LLM_FALLBACK_API_KEY", "LLM_FALLBACK_BASE_URL", "LLM_FALLBACK_MODEL",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("DEEPSEEK_API_KEY", "deepseek-key")
	t.Setenv("LLM_API_KEY", "gemini-key")
	t.Setenv("LLM_BASE_URL", "https://generativelanguage.googleapis.com/v1beta/openai/")
	t.Setenv("LLM_MODEL", "gemini-test")

	cfg := defaultConfig()
	if cfg.LLMBaseURL != "https://api.deepseek.com" || cfg.LLMModel != "deepseek-v4-flash" {
		t.Fatalf("expected DeepSeek primary provider, got %s %s", cfg.LLMBaseURL, cfg.LLMModel)
	}
	if cfg.LLMFallback.BaseURL != "https://generativelanguage.googleapis.com/v1beta/openai/" || cfg.LLMFallback.Model != "gemini-test" {
		t.Fatalf("expected Gemini fallback provider, got %#v", cfg.LLMFallback)
	}
}

func TestSupportsAudioTranscriptionsSkipsTextAndVisionOnlyProviders(t *testing.T) {
	for _, baseURL := range []string{
		"https://api.deepseek.com",
		"https://generativelanguage.googleapis.com/v1beta/openai/",
	} {
		if supportsAudioTranscriptions(baseURL) {
			t.Fatalf("expected audio transcriptions to be disabled for %s", baseURL)
		}
	}
	if !supportsAudioTranscriptions("https://api.openai.com/v1") {
		t.Fatal("expected OpenAI audio transcriptions to remain enabled")
	}
}

func TestNormalizeLLMProviderMode(t *testing.T) {
	tests := map[string]string{
		"":             "direct",
		"direct":       "direct",
		"primary-only": "direct",
		"fallback":     "fallback",
		"failover":     "fallback",
		"unknown":      "",
	}
	for input, want := range tests {
		if got := normalizeLLMProviderMode(input); got != want {
			t.Fatalf("normalizeLLMProviderMode(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeVisionInputMode(t *testing.T) {
	tests := map[string]string{
		"":             "auto",
		"auto":         "auto",
		"native":       "native",
		"multimodal":   "native",
		"local":        "local",
		"apple-vision": "local",
		"unknown":      "",
	}
	for input, want := range tests {
		if got := normalizeVisionInputMode(input); got != want {
			t.Fatalf("normalizeVisionInputMode(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestValidateConfigRequiresConfiguredFallback(t *testing.T) {
	cfg := Config{
		AnalysisLanguage: "auto",
		UseLLM:           true,
		LLMBaseURL:       "http://localhost:9999",
		LLMModel:         "primary-model",
		LLMProviderMode:  "fallback",
	}
	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected incomplete fallback configuration to fail validation")
	}
}
