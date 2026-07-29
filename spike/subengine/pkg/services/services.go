// Package services simulates the mimoproxy/pkg/services package.
// It demonstrates how MiMo would consume auth and client from the
// request context using the shared authctx package.
package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"subengine/pkg/authctx"
)

// Auth mirrors models.Auth in MiMo.
type Auth struct {
	Cookie string
	Ph     string
	Token  string
}

// GlobalHTTPClient mirrors services.GlobalHTTPClient in MiMo (env-based default).
var GlobalHTTPClient = http.DefaultClient

// GetSelectedAuth mirrors services.GetSelectedAuth in MiMo (env-based fallback).
func GetSelectedAuth() Auth {
	return Auth{
		Cookie: `serviceToken="env-token"; userId=env-user; xiaomichatbot_ph="env-ph"`,
		Ph:     "env-ph",
		Token:  "env-token",
	}
}

// ResolveClient returns a non-nil HTTP client. If c is non-nil, it is returned
// as-is. Otherwise it falls back to GlobalHTTPClient, then http.DefaultClient.
// This helper prevents nil-pointer panics and is exported so routes/chat.go
// can use it too (routes is a different package from services).
//
// This is the SAME helper that patch 003-thread-client.patch adds to the
// real mimoproxy/pkg/services/mimo.go.
func ResolveClient(c *http.Client) *http.Client {
	if c != nil {
		return c
	}
	if GlobalHTTPClient != nil {
		return GlobalHTTPClient
	}
	return http.DefaultClient
}

// GetAuthFromContext reads auth and client from the request context.
// If gateway has injected them (per-account), they are returned.
// Otherwise, it falls back to env-based GetSelectedAuth.
//
// This mirrors the function added by patch 002-context-auth.patch in the
// real mimoproxy/pkg/services/mimo.go.
func GetAuthFromContext(ctx context.Context) (Auth, *http.Client) {
	if a, client, ok := authctx.FromContext(ctx); ok {
		return Auth{
			Cookie: a.Cookie,
			Ph:     a.Ph,
			Token:  a.Token,
		}, client
	}
	client := GlobalHTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	return GetSelectedAuth(), client
}

// HandleChat simulates HandleMimoChat in MiMo. It makes a REAL HTTP request
// to upstreamURL using the injected (or fallback) client. This is what makes
// the spike prove that per-account proxy ACTUALLY works — not just that the
// pointer was transferred.
//
// 🔵 Conformance: uses ResolveClient(client), exactly like the patched
// HandleMimoChat in the real project. The old fallback `if client == nil`
// is NOT used here — that would reintroduce the panic bug.
func HandleChat(ctx context.Context, upstreamURL string) (string, error) {
	auth, client := GetAuthFromContext(ctx)
	client = ResolveClient(client) // ✅ conformance with patch 003

	req, err := http.NewRequestWithContext(ctx, "GET", upstreamURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+auth.Token)

	// ✅ This is the critical line: uses the per-account client (with proxy),
	// NOT GlobalHTTPClient. If this used GlobalHTTPClient, per-account proxy
	// would be silently broken (the bug found in SPIKE-PLAN-V3 review).
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("auth.token=%s, upstream_response=%s", auth.Token, string(body)), nil
}
