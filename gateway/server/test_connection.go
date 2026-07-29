// Package server — Connection testing for accounts (proxy + provider).
package server

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"
)

// TestResult holds the outcome of a connection test for one account.
type TestResult struct {
	AccountID       string        `json:"account_id"`
	TestedAt        time.Time     `json:"tested_at"`
	ProxyStatus     string        `json:"proxy_status"`
	ProxyError      string        `json:"proxy_error,omitempty"`
	ProxyLatency    time.Duration `json:"proxy_latency_ms"`
	ProviderStatus  string        `json:"provider_status"`
	ProviderError   string        `json:"provider_error,omitempty"`
	ProviderLatency time.Duration `json:"provider_latency_ms"`
	Overall         string        `json:"overall"`
}

// TestConnection tests proxy reachability and provider credentials.
// Stores the result on the account via SetTestResult.
func (am *AccountManager) TestConnection(id string) (*TestResult, error) {
	am.mu.RLock()
	acct, ok := am.accounts[id]
	am.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("account not found")
	}

	result := &TestResult{AccountID: id, TestedAt: time.Now()}

	// 1. Test proxy (if configured)
	if acct.Proxy != nil {
		proxyStart := time.Now()
		if err := testProxy(acct.Proxy); err != nil {
			result.ProxyStatus = "failed"
			result.ProxyError = err.Error()
			result.ProxyLatency = time.Since(proxyStart)
			result.Overall = "failed"
			am.SetTestResult(id, result)
			return result, nil
		}
		result.ProxyStatus = "ok"
		result.ProxyLatency = time.Since(proxyStart)
	}

	// 2. Test provider
	client := GetHTTPClient(acct.ID, acct.Proxy, 10*time.Second)
	providerStart := time.Now()

	var providerErr error
	switch acct.Provider {
	case ProviderGLM:
		providerErr = testZAIConnection(client, acct.ZaiToken)
	case ProviderMimo:
		providerErr = testMiMoConnection(client, acct.ServiceToken, acct.UserID, acct.XiaomichatPH)
	}

	result.ProviderLatency = time.Since(providerStart)
	if providerErr != nil {
		result.ProviderStatus = "failed"
		result.ProviderError = providerErr.Error()
		result.Overall = "failed"
		am.SetTestResult(id, result)
		return result, nil
	}

	result.ProviderStatus = "ok"
	result.Overall = "ok"
	am.SetTestResult(id, result)
	return result, nil
}

// testProxy verifies the proxy is reachable by making a GET request
// to PROXY_TEST_URL (default: https://api.ipify.org).
func testProxy(cfg *ProxyConfig) error {
	testURL := os.Getenv("PROXY_TEST_URL")
	if testURL == "" {
		testURL = "https://api.ipify.org"
	}

	client := GetHTTPClient("proxy-test", cfg, 10*time.Second)
	resp, err := client.Get(testURL)
	if err != nil {
		return fmt.Errorf("proxy test failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("proxy returned status %d", resp.StatusCode)
	}
	return nil
}

// testZAIConnection verifies the Z.AI token by calling /api/v1/auths/.
func testZAIConnection(client *http.Client, token string) error {
	req, _ := http.NewRequest("GET", "https://chat.z.ai/api/v1/auths/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("z.ai unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return fmt.Errorf("invalid Z.AI token")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("z.ai returned %d", resp.StatusCode)
	}
	return nil
}

// testMiMoConnection verifies MiMo credentials by calling bot/config.
func testMiMoConnection(client *http.Client, token, userID, ph string) error {
	apiURL := fmt.Sprintf("https://aistudio.xiaomimimo.com/open-apis/bot/config?xiaomichatbot_ph=%s",
		url.QueryEscape(ph))

	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("Cookie", fmt.Sprintf(`serviceToken="%s"; userId=%s; xiaomichatbot_ph="%s"`,
		token, userID, ph))

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("xiaomi unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return fmt.Errorf("invalid MiMo credentials")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("xiaomi returned %d", resp.StatusCode)
	}
	return nil
}

// SetTestResult stores the latest test result on the account (thread-safe).
func (am *AccountManager) SetTestResult(id string, result *TestResult) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	if a, ok := am.accounts[id]; ok {
		a.LastTestMu.Lock()
		a.LastTestResult = result
		a.LastTestMu.Unlock()
	}
}

// GetTestResult returns the latest test result (thread-safe).
func (am *AccountManager) GetTestResult(id string) *TestResult {
	am.mu.RLock()
	defer am.mu.RUnlock()

	if a, ok := am.accounts[id]; ok {
		a.LastTestMu.RLock()
		defer a.LastTestMu.RUnlock()
		return a.LastTestResult
	}
	return nil
}
