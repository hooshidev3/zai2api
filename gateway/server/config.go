// Package server — gateway configuration.
package server

import (
        "os"
)

// Config holds all gateway configuration.
type Config struct {
        ListenAddr     string
        GatewayToken   string
        DashboardToken string
        GLMCaptchaDB   string
        AccountsDB     string
        ZAIStrategy    string
        Verbose        bool
        AgentMode      bool
        DefaultModel   string
        Version        string
}

// LoadConfig reads configuration from environment variables.
func LoadConfig() *Config {
        c := &Config{
                ListenAddr:     getenv("LISTEN_ADDR", ":8080"),
                GatewayToken:   getenv("GATEWAY_TOKEN", "sk-merged-default"),
                DashboardToken: os.Getenv("DASHBOARD_TOKEN"),
                GLMCaptchaDB:   getenv("GLM_CAPTCHA_DB", "./data/tokens.sqlite"),
                AccountsDB:     getenv("ACCOUNTS_DB", "./data/accounts.sqlite"),
                ZAIStrategy:    getenv("ZAI_STRATEGY", "round-robin"),
                Verbose:        os.Getenv("VERBOSE") != "" && os.Getenv("VERBOSE") != "0",
                AgentMode:      os.Getenv("AGENT_MODE") != "" && os.Getenv("AGENT_MODE") != "0",
                DefaultModel:   getenv("DEFAULT_MODEL", "glm-5"),
                Version:        "1.0.0",
        }
        return c
}

func getenv(key, def string) string {
        if v := os.Getenv(key); v != "" {
                return v
        }
        return def
}
