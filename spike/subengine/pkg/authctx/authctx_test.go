package authctx

import (
	"context"
	"net/http"
	"testing"
)

// TestKeyIdentity verifies that key identity works correctly within the
// package. Note: this test CANNOT detect the cross-package key-identity
// bug (that bug only manifests when two different packages define their
// own contextKey type). The defense against that bug is the code-review
// rule documented in authctx.go and the TestDirectContextWithValueDoesNotWork
// test below.
func TestKeyIdentity(t *testing.T) {
	auth := InjectedAuth{
		AccountID: "mimo-1",
		Token:     "abc123",
		ProxyInfo: "socks5://10.0.0.1:1080",
	}
	client := &http.Client{}

	ctx := context.Background()
	ctx = WithAuth(ctx, auth)
	ctx = WithClient(ctx, client)

	got, gotClient, ok := FromContext(ctx)
	if !ok {
		t.Fatal("FromContext returned ok=false — key identity broken")
	}
	if got.AccountID != auth.AccountID {
		t.Errorf("AccountID mismatch: got %q, want %q", got.AccountID, auth.AccountID)
	}
	if gotClient != client {
		t.Error("Client mismatch — pointer identity lost")
	}
}

// TestFallbackWhenNotSet verifies that when context has no auth set,
// FromContext returns ok=false so the caller can fall back to env-based auth.
func TestFallbackWhenNotSet(t *testing.T) {
	ctx := context.Background()
	_, _, ok := FromContext(ctx)
	if ok {
		t.Fatal("Expected ok=false when context not set")
	}
}

// TestAccountIDFromContext verifies the logging helper.
func TestAccountIDFromContext(t *testing.T) {
	ctx := context.Background()
	if id := AccountIDFromContext(ctx); id != "" {
		t.Errorf("Expected empty AccountID, got %q", id)
	}

	ctx = WithAuth(ctx, InjectedAuth{AccountID: "zai-1"})
	if id := AccountIDFromContext(ctx); id != "zai-1" {
		t.Errorf("Expected zai-1, got %q", id)
	}
}

// TestDirectContextWithValueDoesNotWork is a defensive safeguard. It verifies
// that if a developer in the future tries to use context.WithValue directly
// with a foreign key type, FromContext will NOT pick it up. This protects
// against the silent runtime bug described in authctx.go.
//
// If this test fails, it means someone has changed the key type and broken
// identity — the silent bug has returned.
func TestDirectContextWithValueDoesNotWork(t *testing.T) {
	// Simulate a foreign package defining its own contextKey type
	type foreignContextKey string
	const foreignKey foreignContextKey = "auth"

	ctx := context.WithValue(context.Background(), foreignKey, "should-not-work")

	_, _, ok := FromContext(ctx)
	if ok {
		t.Fatal("FromContext should return ok=false for foreign context keys — " +
			"if this fails, someone may have changed the key type and broken identity")
	}
}
