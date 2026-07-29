// Package server — gateway configuration.
package server

import (
	"os"
	"strconv"
)

// Config holds all gateway configuration.
type Config struct {
	ListenAddr     string
	GatewayToken   string
	DashboardToken string
	GLMCaptchaDB   string
	Verbose        bool
	AgentMode      bool
	DefaultModel   string
}

// LoadConfig reads configuration from environment variables.
func LoadConfig() *Config {
	c := &Config{
		ListenAddr:     getenv("LISTEN_ADDR", ":8080"),
		GatewayToken:   getenv("GATEWAY_TOKEN", "sk-merged-default"),
		DashboardToken: os.Getenv("DASHBOARD_TOKEN"), // empty = localhost only
		GLMCaptchaDB:   getenv("GLM_CAPTCHA_DB", "./data/tokens.sqlite"),
		Verbose:        os.Getenv("VERBOSE") != "" && os.Getenv("VERBOSE") != "0",
		AgentMode:      os.Getenv("AGENT_MODE") != "" && os.Getenv("AGENT_MODE") != "0",
		DefaultModel:   getenv("DEFAULT_MODEL", "glm-5"),
	}
	return c
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func atoi(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}
