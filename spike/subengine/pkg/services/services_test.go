package services

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"subengine/pkg/authctx"
)

// TestResolveClient verifies the helper that prevents nil-pointer panics.
//
// ⚠️ This test mutates global state (GlobalHTTPClient). It must NOT run
// with t.Parallel(). The defer ensures GlobalHTTPClient is restored even
// if an assertion fails midway.
func TestResolveClient(t *testing.T) {
	// ⚠️ Not parallel — mutates GlobalHTTPClient
	originalGlobal := GlobalHTTPClient
	defer func() { GlobalHTTPClient = originalGlobal }()

	// 1. If c is non-nil, return c as-is
	customClient := &http.Client{Timeout: 5 * time.Second}
	if got := ResolveClient(customClient); got != customClient {
		t.Error("ResolveClient should return the same client if non-nil")
	}

	// 2. If c is nil and GlobalHTTPClient is non-nil, return GlobalHTTPClient
	GlobalHTTPClient = &http.Client{Timeout: 10 * time.Second}
	if got := ResolveClient(nil); got != GlobalHTTPClient {
		t.Error("ResolveClient should return GlobalHTTPClient if c is nil")
	}

	// 3. If both are nil, return http.DefaultClient
	GlobalHTTPClient = nil
	if got := ResolveClient(nil); got != http.DefaultClient {
		t.Error("ResolveClient should return http.DefaultClient if both are nil")
	}
}

// TestHandleChatUsesInjectedClient is THE critical test of the spike.
// It proves that when gateway injects a per-account client (with a proxy),
// HandleChat ACTUALLY uses that client to make the request — not GlobalHTTPClient.
//
// Setup:
//   - upstream: httptest.Server that counts requests (simulates Xiaomi API)
//   - proxy:    httptest.Server that forwards to upstream and counts requests
//   - injectedClient: http.Client configured to use the proxy
//
// Assertion:
//   - proxyRequests > 0  (request went THROUGH the proxy)
//   - upstreamRequests > 0  (proxy forwarded to upstream)
//   - result contains the injected token (auth propagated)
//   - result contains the upstream response (round-trip succeeded)
//
// If proxyRequests == 0, per-account proxy is BROKEN — the spike has failed
// and the entire design must be revisited before Phase 1.
func TestHandleChatUsesInjectedClient(t *testing.T) {
	// 1. upstream mock server — simulates aistudio.xiaomimimo.com
	upstreamRequests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamRequests++
		// Echo back the auth header so we can verify it propagated
		token := r.Header.Get("Authorization")
		io.WriteString(w, "upstream-ok token="+strings.TrimPrefix(token, "Bearer "))
	}))
	defer upstream.Close()

	// 2. proxy mock server — simulates SOCKS5/HTTP proxy
	proxyRequests := 0
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyRequests++
		// Proxy forwards the request to upstream
		forwardReq, _ := http.NewRequest(r.Method, upstream.URL+r.URL.Path, r.Body)
		forwardReq.Header = r.Header
		resp, err := http.DefaultClient.Do(forwardReq)
		if err != nil {
			t.Errorf("proxy forward failed: %v", err)
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}))
	defer proxy.Close()

	// 3. Build an HTTP client that uses the proxy mock (like per-account proxy)
	proxyURL, _ := url.Parse(proxy.URL)
	injectedClient := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
	}

	// 4. Inject auth + client into context (this is what gateway does)
	ctx := authctx.WithAuth(context.Background(), authctx.InjectedAuth{
		AccountID: "mimo-1",
		Token:     "token1-abc123",
		ProxyInfo: proxy.URL,
	})
	ctx = authctx.WithClient(ctx, injectedClient)

	// 5. Call HandleChat — it should make a REAL request through the proxy
	result, err := HandleChat(ctx, upstream.URL+"/test")
	if err != nil {
		t.Fatalf("HandleChat failed: %v", err)
	}

	// 6. ✅ CRITICAL assertions
	if proxyRequests == 0 {
		t.Error("❌ Proxy was NOT used — request went directly to upstream. " +
			"Per-account proxy is BROKEN. Redesign needed before Phase 1.")
	}
	if upstreamRequests == 0 {
		t.Error("❌ Upstream was NOT called — proxy didn't forward the request.")
	}
	if !strings.Contains(result, "token1-abc123") {
		t.Errorf("❌ Auth token not propagated: %s", result)
	}
	if !strings.Contains(result, "upstream-ok") {
		t.Errorf("❌ Upstream response not in result: %s", result)
	}

	t.Logf("✅ proxy_requests=%d, upstream_requests=%d", proxyRequests, upstreamRequests)
	t.Logf("✅ result: %s", result)
}

// TestHandleChatFallbackToGlobalClient verifies backward compatibility:
// when gateway does NOT inject auth/client, HandleChat falls back to
// env-based GetSelectedAuth and GlobalHTTPClient.
func TestHandleChatFallbackToGlobalClient(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "env-ok")
	}))
	defer upstream.Close()

	// No injection — simulate env-based fallback
	ctx := context.Background()

	result, err := HandleChat(ctx, upstream.URL+"/test")
	if err != nil {
		t.Fatalf("HandleChat failed: %v", err)
	}

	// Must use env-token (fallback)
	if !strings.Contains(result, "env-token") {
		t.Errorf("❌ Expected env-token fallback, got: %s", result)
	}
	if !strings.Contains(result, "env-ok") {
		t.Errorf("❌ Upstream response not received: %s", result)
	}
}
