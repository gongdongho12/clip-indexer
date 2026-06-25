package media

import (
	"net/http"
	"net/url"
	"testing"
)

func TestIsBlockedLoopbackProxy(t *testing.T) {
	tests := []struct {
		raw  string
		want bool
	}{
		{raw: "http://127.0.0.1:9", want: true},
		{raw: "http://localhost:9", want: true},
		{raw: "http://[::1]:9", want: true},
		{raw: "http://127.0.0.1:8080", want: false},
		{raw: "http://proxy.example:9", want: false},
	}

	for _, tt := range tests {
		proxyURL, err := url.Parse(tt.raw)
		if err != nil {
			t.Fatalf("parse proxy URL: %v", err)
		}
		if got := isBlockedLoopbackProxy(proxyURL); got != tt.want {
			t.Fatalf("isBlockedLoopbackProxy(%q) = %v, want %v", tt.raw, got, tt.want)
		}
	}
}

func TestLLMProxyFromEnvironmentSkipsBlockedEnvProxy(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:9")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:9")
	t.Setenv("ALL_PROXY", "http://127.0.0.1:9")
	t.Setenv("NO_PROXY", "")

	req, err := http.NewRequest(http.MethodPost, "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	proxyURL, err := llmProxyFromEnvironment(req)
	if err != nil {
		t.Fatalf("llmProxyFromEnvironment returned error: %v", err)
	}
	if proxyURL != nil {
		t.Fatalf("expected blocked loopback proxy to be ignored, got %s", proxyURL)
	}
}
