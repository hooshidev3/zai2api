#!/usr/bin/env python3
"""
Add GetAuthFromContext function to mimoproxy/pkg/services/mimo.go.

This script is idempotent: if the function already exists, it does nothing.
"""
import sys
import re

def main():
    if len(sys.argv) != 2:
        print("Usage: add-getauthfromcontext.py <mimo.go>", file=sys.stderr)
        sys.exit(1)
    
    path = sys.argv[1]
    with open(path, 'r') as f:
        content = f.read()
    
    if 'func GetAuthFromContext' in content:
        print(f"  GetAuthFromContext already exists in {path}, skipping")
        return
    
    # Add imports if not present
    if '"context"' not in content:
        # Add after the first import line
        content = re.sub(
            r'(import \(\s*\n)',
            r'\1\t"context"\n\t"mimoproxy/pkg/authctx"\n',
            content,
            count=1
        )
    elif '"mimoproxy/pkg/authctx"' not in content:
        content = re.sub(
            r'("context"\n)',
            r'\1\t"mimoproxy/pkg/authctx"\n',
            content,
            count=1
        )
    
    # Add the function at the end of the file
    new_func = '''

// GetAuthFromContext reads auth and client from request context.
// If gateway has injected them (per-account), they are returned.
// Otherwise falls back to env-based GetSelectedAuth() and GlobalHTTPClient.
func GetAuthFromContext(ctx context.Context) (models.Auth, *http.Client) {
	if a, client, ok := authctx.FromContext(ctx); ok {
		return models.Auth{
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

// ResolveClient returns a non-nil HTTP client. If c is non-nil, returns c.
// Otherwise falls back to GlobalHTTPClient, then http.DefaultClient.
// Prevents nil-pointer panics. Exported so routes/chat.go can use it too.
func ResolveClient(c *http.Client) *http.Client {
	if c != nil {
		return c
	}
	if GlobalHTTPClient != nil {
		return GlobalHTTPClient
	}
	return http.DefaultClient
}
'''
    
    content = content.rstrip() + new_func
    
    with open(path, 'w') as f:
        f.write(content)
    
    print(f"  Added GetAuthFromContext and ResolveClient to {path}")

if __name__ == '__main__':
    main()
