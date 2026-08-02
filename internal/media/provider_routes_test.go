package media

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConfigForVisionAnalysisRoutesDeepSeekToGemini(t *testing.T) {
	cfg := multimodalRoutingTestConfig()
	routed := configForVisionAnalysis(cfg)
	if routed.LLMModel != "gemini-3.6-flash" || !isGeminiHosted(routed.LLMBaseURL) {
		t.Fatalf("unexpected vision provider: %s %s", routed.LLMBaseURL, routed.LLMModel)
	}
	if routed.LLMProviderMode != "direct" || llmProviderConfigured(routed.LLMFallback) {
		t.Fatalf("vision provider should be isolated from text failover: %#v", routed)
	}
}

func TestConfigForVisionAnalysisPreservesExplicitLocalMode(t *testing.T) {
	cfg := multimodalRoutingTestConfig()
	cfg.VisionInputMode = "local"
	routed := configForVisionAnalysis(cfg)
	if !isDeepSeekHosted(routed.LLMBaseURL) {
		t.Fatalf("explicit local mode should keep the text provider, got %s", routed.LLMBaseURL)
	}
}

func TestConfigForAudioAnalysisRoutesDeepSeekToGemini(t *testing.T) {
	routed, supported := configForAudioAnalysis(multimodalRoutingTestConfig())
	if !supported || routed.LLMModel != "gemini-3.6-flash" || !isGeminiHosted(routed.LLMBaseURL) {
		t.Fatalf("unexpected audio provider: supported=%v config=%#v", supported, routed)
	}
	if audioAnalysisModel(routed) != "gemini-3.6-flash" {
		t.Fatalf("audio model = %q", audioAnalysisModel(routed))
	}
}

func TestBuildGeminiAudioRequestIncludesInlineWAV(t *testing.T) {
	request := buildGeminiAudioRequest(
		Config{LLMModel: "gemini-3.6-flash", AnalysisLanguage: "en"},
		[]byte(`{"source_path":"/tmp/source.mp4"}`),
		[]byte("wav-data"),
	)
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	body := string(encoded)
	for _, expected := range []string{"gemini-3.6-flash", "input_audio", "d2F2LWRhdGE=", "json_object"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("request does not contain %q: %s", expected, body)
		}
	}
}

func multimodalRoutingTestConfig() Config {
	return Config{
		LLMBaseURL:      "https://api.deepseek.com",
		LLMModel:        "deepseek-v4-flash",
		LLMProviderMode: "fallback",
		VisionInputMode: "auto",
		LLMFallback: LLMProviderConfig{
			BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai/",
			Model:   "gemini-3.6-flash",
		},
	}
}
