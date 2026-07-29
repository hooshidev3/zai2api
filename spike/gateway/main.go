// Package main is the gateway in the spike. It simulates the merged-proxy-v2
// gateway: selects an account round-robin, builds a per-account HTTP client
// (placeholder here — real proxy mock is in services_test.go), injects auth
// and client into the request context, and forwards to the sub-engine.
//
// Note: This gateway uses a placeholder client (no real proxy). The actual
// proof that per-account proxy works is in:
//   spike/subengine/pkg/services/services_test.go :: TestHandleChatUsesInjectedClient
//
// The curl test below only proves round-robin auth selection + fallback.
// See README.md for details on what each test proves.
package main

import (
        "fmt"
        "net/http"
        "sync/atomic"

        "subengine/pkg/authctx"
        "subengine/pkg/services"

        "github.com/gin-gonic/gin"
)

var rrIdx atomic.Int64

type account struct {
        ID        string
        Token     string
        ProxyInfo string
}

// mockAccounts simulates the AccountManager's account pool.
var mockAccounts = []account{
        {ID: "mimo-1", Token: "token1-abc123", ProxyInfo: "socks5://10.0.0.1:1080"},
        {ID: "mimo-2", Token: "token2-xyz789", ProxyInfo: ""},
}

// subEngine is built once and reused — this is exactly the pattern the real
// gateway uses (mimoEngine := gin.New(); routes.RegisterChatRoutes(mimoEngine, nil))
var subEngine = buildSubEngine()

func buildSubEngine() *gin.Engine {
        r := gin.New()
        r.POST("/test", func(c *gin.Context) {
                // Simulates handleChatCompletions in MiMo.
                // upstream URL comes from query (for testing); defaults to a real URL.
                upstreamURL := c.Query("upstream")
                if upstreamURL == "" {
                        upstreamURL = "https://aistudio.xiaomimimo.com/open-apis/bot/chat"
                }

                // HandleChat makes a REAL request using the injected client.
                result, err := services.HandleChat(c.Request.Context(), upstreamURL)
                if err != nil {
                        c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
                        return
                }
                c.JSON(http.StatusOK, gin.H{"result": result})
        })
        return r
}

func main() {
        gin.SetMode(gin.ReleaseMode)
        r := gin.New()

        r.POST("/test", handleTest)
        r.GET("/auth-info", handleAuthInfo)

        fmt.Println("gateway on :9999")
        fmt.Println("  POST /test        — round-robin auth (no real proxy in gateway)")
        fmt.Println("  GET  /auth-info   — fallback to env-based (no injection)")
        fmt.Println()
        fmt.Println("⚠️  Per-account proxy is proven in unit test, not curl test:")
        fmt.Println("    cd spike/subengine && go test -v -run TestHandleChatUsesInjectedClient")

        if err := r.Run(":9999"); err != nil {
                panic(err)
        }
}

// handleTest simulates forwardToMiMo in the real gateway.
func handleTest(c *gin.Context) {
        // 1. Select account round-robin
        idx := int(rrIdx.Add(1)-1) % len(mockAccounts)
        selected := mockAccounts[idx]

        // 2. Build per-account HTTP client.
        // In the real gateway, this would be:
        //   client := GetHTTPClient(acct.ID, acct.Proxy, 0)
        // Here we use a placeholder — the real proxy test is in services_test.go.
        client := &http.Client{}

        // 3. Inject auth + client into request context (NOT gin context —
        //    gin.Context does not survive ServeHTTP, but *http.Request does).
        injected := authctx.InjectedAuth{
                AccountID: selected.ID,
                Token:     selected.Token,
                ProxyInfo: selected.ProxyInfo,
        }
        ctx := authctx.WithAuth(c.Request.Context(), injected)
        ctx = authctx.WithClient(ctx, client)
        c.Request = c.Request.WithContext(ctx)

        fmt.Printf("[gateway] account=%s token=%s proxy=%s\n",
                selected.ID, selected.Token[:10]+"...", selected.ProxyInfo)

        // 4. Forward to sub-engine — exactly like mimoEngine.ServeHTTP in real gateway
        subEngine.ServeHTTP(c.Writer, c.Request)
        c.Abort()
}

// handleAuthInfo tests fallback: no auth injected, sub-engine should use env-based.
// We rewrite the request to POST /test (the only route in subEngine) but
// skip the auth-injection step that handleTest does. This lets us verify
// that subEngine falls back to GetSelectedAuth() (env-based) when gateway
// has not injected anything.
func handleAuthInfo(c *gin.Context) {
        // Rewrite to POST /test so subEngine's only route matches
        c.Request.Method = http.MethodPost
        c.Request.URL.Path = "/test"
        // Do NOT inject auth — this is the whole point of the fallback test
        subEngine.ServeHTTP(c.Writer, c.Request)
        c.Abort()
}
