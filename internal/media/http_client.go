package media

import (
	"net/http"
	"net/url"
)

var llmHTTPClient = &http.Client{
	Transport: &http.Transport{
		Proxy: llmProxyFromEnvironment,
	},
}

func llmProxyFromEnvironment(req *http.Request) (*url.URL, error) {
	proxyURL, err := http.ProxyFromEnvironment(req)
	if err != nil || proxyURL == nil {
		return proxyURL, err
	}
	if isBlockedLoopbackProxy(proxyURL) {
		return nil, nil
	}
	return proxyURL, nil
}

func isBlockedLoopbackProxy(proxyURL *url.URL) bool {
	if proxyURL == nil {
		return false
	}
	host := proxyURL.Hostname()
	port := proxyURL.Port()
	return port == "9" && (host == "127.0.0.1" || host == "localhost" || host == "::1")
}
