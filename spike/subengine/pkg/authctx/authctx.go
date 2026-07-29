// Package authctx provides a shared, dependency-free way to inject
// authentication and HTTP client through request context.
//
// ⚠️ Code Review Rule:
// Never use context.WithValue directly for auth. Always use WithAuth/FromContext
// to guarantee key identity. Direct use of context.WithValue causes a silent
// runtime bug that may go undetected for weeks.
//
// Example WRONG:
//   type contextKey string
//   const authKey contextKey = "mimo_account"
//   ctx = context.WithValue(ctx, authKey, account)  // ❌
//
// Example CORRECT:
//   ctx = authctx.WithAuth(ctx, injectedAuth)  // ✅
//
// This package is a leaf package — it has no dependencies on gateway or
// mimoproxy. Both gateway and mimoproxy/pkg/services import it to share
// the same context key identity without creating an import cycle.
package authctx

import (
	"context"
	"net/http"
)

// contextKey is defined ONLY in this package. Because it is unexported and
// defined once, its type identity is shared across all importers — this is
// what makes ctx.Value() work correctly across package boundaries.
type contextKey int

const (
	authKey  contextKey = iota // first key
	clientKey                  // second key
)

// InjectedAuth contains only the fields MiMo needs — no reference to the
// real Account type. This is what breaks the import cycle: services does
// not need to import gateway to read auth.
type InjectedAuth struct {
	Cookie string
	Ph     string
	Token  string

	// Optional fields for observability (logging, metrics)
	AccountID string
	Provider  string
	ProxyInfo string
}

// WithAuth places auth into the context. Gateway calls this before
// forwarding to the sub-engine.
func WithAuth(ctx context.Context, a InjectedAuth) context.Context {
	return context.WithValue(ctx, authKey, a)
}

// WithClient places a per-account HTTP client into the context.
func WithClient(ctx context.Context, c *http.Client) context.Context {
	return context.WithValue(ctx, clientKey, c)
}

// FromContext reads auth and client from the context. If not set,
// ok = false is returned (caller must fall back).
func FromContext(ctx context.Context) (InjectedAuth, *http.Client, bool) {
	a, ok := ctx.Value(authKey).(InjectedAuth)
	if !ok {
		return InjectedAuth{}, nil, false
	}
	c, _ := ctx.Value(clientKey).(*http.Client)
	return a, c, true
}

// AccountIDFromContext returns only the AccountID (for logging in middleware).
func AccountIDFromContext(ctx context.Context) string {
	if a, ok := ctx.Value(authKey).(InjectedAuth); ok {
		return a.AccountID
	}
	return ""
}
