// Package server — Per-account HTTP client with proxy support (HTTP/HTTPS/SOCKS5).
//
// Clients are cached by (accountID, proxyURL) for connection pooling.
// When a proxy changes, call InvalidateClient(accountID) to clear the cache.
package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

var clientCache sync.Map // map[string]*http.Client

// GetHTTPClient returns a cached *http.Client for the given account.
// If cfg is non-nil, the client is configured to use the proxy.
// timeout=0 means no timeout (for streaming).
func GetHTTPClient(accountID string, cfg *ProxyConfig, timeout time.Duration) *http.Client {
	cacheKey := accountID
	if cfg != nil {
		cacheKey = fmt.Sprintf("%s|%s", accountID, cfg.URL())
	}

	if c, ok := clientCache.Load(cacheKey); ok {
		return c.(*http.Client)
	}

	transport := buildTransport(cfg)
	client := &http.Client{
		Transport: transport,
	}
	if timeout > 0 {
		client.Timeout = timeout
	}

	clientCache.Store(cacheKey, client)
	return client
}

// buildTransport creates an http.Transport with the proxy configured.
func buildTransport(cfg *ProxyConfig) *http.Transport {
	t := &http.Transport{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		MaxConnsPerHost:       20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
	}

	if cfg == nil {
		return t
	}

	switch cfg.Type {
	case "http", "https":
		proxyURL, err := url.Parse(cfg.URL())
		if err == nil {
			t.Proxy = http.ProxyURL(proxyURL)
		}
	case "socks5":
		var auth *proxy.Auth
		if cfg.Username != "" {
			auth = &proxy.Auth{
				User:     cfg.Username,
				Password: cfg.Password,
			}
		}
		dialer, err := proxy.SOCKS5("tcp",
			fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
			auth,
			&net.Dialer{Timeout: 30 * time.Second},
		)
		if err == nil {
			// SOCKS5 dialer from x/net/proxy implements ContextDialer.
			// Prefer DialContext for proper cancellation support.
			if cd, ok := dialer.(proxy.ContextDialer); ok {
				t.DialContext = cd.DialContext
			} else {
				t.Dial = dialer.Dial
			}
		}
	}

	return t
}

// InvalidateClient removes cached HTTP clients for an account.
// Call this when an account's proxy config changes.
func InvalidateClient(accountID string) {
	clientCache.Range(func(k, v any) bool {
		if key, ok := k.(string); ok {
			if key == accountID || strings.HasPrefix(key, accountID+"|") {
				clientCache.Delete(k)
			}
		}
		return true
	})
}

// ResolveClient returns a non-nil HTTP client. If c is non-nil, returns c.
// Otherwise returns http.DefaultClient. Used as a safety net to prevent
// nil-pointer panics.
func ResolveClient(c *http.Client) *http.Client {
	if c != nil {
		return c
	}
	return http.DefaultClient
}

// Compile-time assertion that context is used (for DialContext signature).
var _ = context.Background
