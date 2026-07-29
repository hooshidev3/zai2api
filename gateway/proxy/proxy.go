// Package proxy — HTTP client management for per-account proxy support.
//
// In Phase 3, this will manage per-account HTTP clients with proxy
// configuration (HTTP/HTTPS/SOCKS5). For now, it provides the
// ResolveClient helper used by services.
package proxy

import "net/http"

// ResolveClient returns a non-nil HTTP client. If c is non-nil, returns c.
// Otherwise returns http.DefaultClient.
//
// This is a gateway-level convenience; MiMo has its own ResolveClient
// in pkg/services/mimo.go that also checks GlobalHTTPClient.
func ResolveClient(c *http.Client) *http.Client {
	if c != nil {
		return c
	}
	return http.DefaultClient
}
