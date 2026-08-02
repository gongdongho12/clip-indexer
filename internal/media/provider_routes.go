package media

import "strings"

func configForVisionAnalysis(cfg Config) Config {
	if normalizeVisionInputMode(cfg.VisionInputMode) == "local" {
		return cfg
	}
	primary := primaryLLMProvider(cfg)
	if providerSupportsNativeVision(primary) {
		return cfg
	}
	if providerSupportsNativeVision(cfg.LLMFallback) {
		return configWithProvider(cfg, cfg.LLMFallback)
	}
	return cfg
}

func configForAudioAnalysis(cfg Config) (Config, bool) {
	primary := primaryLLMProvider(cfg)
	if providerSupportsAudio(primary) {
		return cfg, true
	}
	if providerSupportsAudio(cfg.LLMFallback) {
		return configWithProvider(cfg, cfg.LLMFallback), true
	}
	return cfg, false
}

func supportsConfiguredAudioAnalysis(cfg Config) bool {
	_, supported := configForAudioAnalysis(cfg)
	return supported
}

func primaryLLMProvider(cfg Config) LLMProviderConfig {
	return LLMProviderConfig{
		BaseURL: cfg.LLMBaseURL,
		APIKey:  cfg.LLMAPIKey,
		Model:   cfg.LLMModel,
	}
}

func configWithProvider(cfg Config, provider LLMProviderConfig) Config {
	cfg.LLMBaseURL = provider.BaseURL
	cfg.LLMAPIKey = provider.APIKey
	cfg.LLMModel = provider.Model
	cfg.LLMFallback = LLMProviderConfig{}
	cfg.LLMProviderMode = "direct"
	return cfg
}

func providerSupportsNativeVision(provider LLMProviderConfig) bool {
	return llmProviderConfigured(provider) && !isDeepSeekHosted(provider.BaseURL)
}

func providerSupportsAudio(provider LLMProviderConfig) bool {
	if !llmProviderConfigured(provider) {
		return false
	}
	return isGeminiHosted(provider.BaseURL) || supportsAudioTranscriptions(provider.BaseURL)
}

func isGeminiHosted(baseURL string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(baseURL)), "generativelanguage.googleapis.com")
}
