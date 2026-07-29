// Package main — zai2api gateway entry point.
//
// Unified gateway for GLM-Free-API (Z.AI) and mimo-ai-proxy (Xiaomi MiMo).
// Single endpoint, single auth token, multi-account, per-account proxy.
package main

import (
        "context"
        "log"
        "os"
        "os/signal"
        "syscall"
        "time"

        "zai2api/gateway/server"
)

func main() {
        cfg := server.LoadConfig()

        srv, err := server.NewServer(cfg)
        if err != nil {
                log.Fatalf("Failed to create server: %v", err)
        }
        defer srv.Close()

        // Start HTTP server in background
        go func() {
                log.Printf("=== Unified AI Gateway ===")
                log.Printf("Listening on %s", cfg.ListenAddr)
                log.Printf("Gateway Token: %s", maskToken(cfg.GatewayToken))
                log.Printf("GLM: %s", cfg.GLMCaptchaDB)
                log.Printf("MiMo: ready (uses env vars SERVICE_TOKENS, USER_IDS, XIAOMI_CHATBOT_PHS)")
                if err := srv.Run(cfg.ListenAddr); err != nil {
                        log.Fatalf("server error: %v", err)
                }
        }()

        // Wait for interrupt signal
        quit := make(chan os.Signal, 1)
        signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
        <-quit

        log.Println("Shutting down...")
        ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
        defer cancel()
        _ = srv.Shutdown(ctx)
        log.Println("Server exited")
}

func maskToken(t string) string {
        if len(t) < 12 {
                return "***"
        }
        return t[:6] + "..." + t[len(t)-4:]
}
