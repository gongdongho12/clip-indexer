package media

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCallChatCompletionFallsBackWithFallbackModel(t *testing.T) {
	primaryCalls := 0
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryCalls++
		http.Error(w, "primary unavailable", http.StatusServiceUnavailable)
	}))
	defer primary.Close()

	fallbackCalls := 0
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackCalls++
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode fallback request: %v", err)
			return
		}
		if request["model"] != "fallback-model" {
			t.Errorf("fallback model = %v", request["model"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]string{"content": `{"items":[]}`},
			}},
		})
	}))
	defer fallback.Close()

	cfg := Config{
		LLMBaseURL:      primary.URL,
		LLMAPIKey:       "primary-key",
		LLMModel:        "primary-model",
		LLMProviderMode: "fallback",
		LLMFallback: LLMProviderConfig{
			BaseURL: fallback.URL,
			APIKey:  "fallback-key",
			Model:   "fallback-model",
		},
	}
	result, err := callChatCompletion(context.Background(), cfg, map[string]any{
		"model": "caller-model",
		"messages": []map[string]string{{
			"role": "user", "content": "Return JSON.",
		}},
	})
	if err != nil {
		t.Fatalf("callChatCompletion: %v", err)
	}
	if result.Model != "fallback-model" || result.Content != `{"items":[]}` {
		t.Fatalf("unexpected fallback result: %#v", result)
	}
	if primaryCalls != 1 || fallbackCalls != 1 {
		t.Fatalf("calls primary=%d fallback=%d", primaryCalls, fallbackCalls)
	}
}

func TestCallChatCompletionDirectDoesNotUseFallback(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "primary unavailable", http.StatusServiceUnavailable)
	}))
	defer primary.Close()

	fallbackCalls := 0
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackCalls++
		w.WriteHeader(http.StatusOK)
	}))
	defer fallback.Close()

	cfg := Config{
		LLMBaseURL:      primary.URL,
		LLMModel:        "primary-model",
		LLMProviderMode: "direct",
		LLMFallback: LLMProviderConfig{
			BaseURL: fallback.URL,
			Model:   "fallback-model",
		},
	}
	if _, err := callChatCompletion(context.Background(), cfg, map[string]any{}); err == nil {
		t.Fatal("expected primary error")
	}
	if fallbackCalls != 0 {
		t.Fatalf("fallback calls = %d, want 0", fallbackCalls)
	}
}

func TestLLMHTTPErrorSummarizesDeepSeekImageRejection(t *testing.T) {
	err := llmHTTPError(
		LLMProviderConfig{BaseURL: "https://api.deepseek.com"},
		http.StatusBadRequest,
		[]byte("{\"error\":{\"message\":\"unknown variant `image_url`, expected `text`\"}}"),
	)
	want := "DeepSeek rejected image input (HTTP 400): the configured endpoint currently accepts text messages only"
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err, want)
	}
}
