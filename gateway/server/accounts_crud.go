// Package server — Account CRUD operations (Create, Read, Update, Delete, Toggle).
package server

import (
        "database/sql"
        "fmt"
        "time"
)

// CreateAccountRequest is the JSON body for POST /api/v1/accounts.
type CreateAccountRequest struct {
        ID           string       `json:"id" binding:"required"`
        Provider     ProviderType `json:"provider" binding:"required"`
        DisplayName  string       `json:"display_name"`
        Notes        string       `json:"notes"`
        ZaiToken     string       `json:"zai_token"`
        ServiceToken string       `json:"service_token"`
        UserID       string       `json:"user_id"`
        XiaomichatPH string       `json:"xiaomichatbot_ph"`
        Proxy        *ProxyConfig `json:"proxy"`
}

// UpdateAccountRequest is the JSON body for PUT /api/v1/accounts/:id.
// Only non-empty fields are updated.
type UpdateAccountRequest struct {
        DisplayName  string       `json:"display_name"`
        Notes        string       `json:"notes"`
        ZaiToken     string       `json:"zai_token"`
        ServiceToken string       `json:"service_token"`
        UserID       string       `json:"user_id"`
        XiaomichatPH string       `json:"xiaomichatbot_ph"`
        Proxy        *ProxyConfig `json:"proxy"`
}

// AccountExportDTO is the JSON form for export (includes full tokens).
type AccountExportDTO struct {
        ID           string       `json:"id"`
        Provider     ProviderType `json:"provider"`
        DisplayName  string       `json:"display_name"`
        ZaiToken     string       `json:"zai_token,omitempty"`
        ServiceToken string       `json:"service_token,omitempty"`
        UserID       string       `json:"user_id,omitempty"`
        XiaomichatPH string       `json:"xiaomichatbot_ph,omitempty"`
        Proxy        *ProxyConfig `json:"proxy,omitempty"`
}

// Create validates and inserts a new account.
func (am *AccountManager) Create(req CreateAccountRequest) (*Account, error) {
        if err := am.validate(req); err != nil {
                return nil, err
        }

        a := &Account{
                ID:           req.ID,
                Provider:     req.Provider,
                DisplayName:  req.DisplayName,
                Notes:        req.Notes,
                ZaiToken:     req.ZaiToken,
                ServiceToken: req.ServiceToken,
                UserID:       req.UserID,
                XiaomichatPH: req.XiaomichatPH,
                Proxy:        req.Proxy,
                CreatedAt:    time.Now(),
                UpdatedAt:    time.Now(),
        }
        a.Enabled.Store(true)

        if err := am.saveToDB(a); err != nil {
                return nil, fmt.Errorf("save to db: %w", err)
        }

        am.mu.Lock()
        if _, exists := am.accounts[a.ID]; !exists {
                am.order = append(am.order, a.ID)
        }
        am.accounts[a.ID] = a
        am.mu.Unlock()

        // Invalidate any cached HTTP client for this account ID (in case of re-create)
        InvalidateClient(a.ID)

        return a, nil
}

// Update modifies an existing account (only non-empty fields).
func (am *AccountManager) Update(id string, req UpdateAccountRequest) (*Account, error) {
        am.mu.Lock()
        defer am.mu.Unlock()

        acct, ok := am.accounts[id]
        if !ok {
                return nil, fmt.Errorf("account not found")
        }

        if req.DisplayName != "" {
                acct.DisplayName = req.DisplayName
        }
        if req.Notes != "" {
                acct.Notes = req.Notes
        }
        if req.ZaiToken != "" {
                acct.ZaiToken = req.ZaiToken
        }
        if req.ServiceToken != "" {
                acct.ServiceToken = req.ServiceToken
        }
        if req.UserID != "" {
                acct.UserID = req.UserID
        }
        if req.XiaomichatPH != "" {
                acct.XiaomichatPH = req.XiaomichatPH
        }
        if req.Proxy != nil {
                acct.Proxy = req.Proxy
        }

        acct.UpdatedAt = time.Now()

        if err := am.saveToDB(acct); err != nil {
                return nil, err
        }

        // Invalidate cached HTTP client (proxy may have changed)
        InvalidateClient(acct.ID)

        return acct, nil
}

// Delete removes an account from DB and memory.
func (am *AccountManager) Delete(id string) error {
        am.mu.Lock()
        defer am.mu.Unlock()

        if _, ok := am.accounts[id]; !ok {
                return fmt.Errorf("account not found")
        }

        if _, err := am.db.Exec("DELETE FROM accounts WHERE id = ?", id); err != nil {
                return err
        }

        delete(am.accounts, id)
        for i, oid := range am.order {
                if oid == id {
                        am.order = append(am.order[:i], am.order[i+1:]...)
                        break
                }
        }

        InvalidateClient(id)
        return nil
}

// Toggle enables or disables an account.
func (am *AccountManager) Toggle(id string, enabled bool) error {
        am.mu.Lock()
        defer am.mu.Unlock()

        acct, ok := am.accounts[id]
        if !ok {
                return fmt.Errorf("account not found")
        }

        acct.Enabled.Store(enabled)
        acct.UpdatedAt = time.Now()

        return am.saveToDB(acct)
}

func (am *AccountManager) validate(req CreateAccountRequest) error {
        if req.ID == "" {
                return fmt.Errorf("id is required")
        }
        if req.Provider != ProviderGLM && req.Provider != ProviderMimo {
                return fmt.Errorf("provider must be 'glm' or 'mimo'")
        }
        if req.Provider == ProviderGLM && req.ZaiToken == "" {
                return fmt.Errorf("zai_token is required for GLM accounts")
        }
        if req.Provider == ProviderMimo {
                if req.ServiceToken == "" || req.UserID == "" || req.XiaomichatPH == "" {
                        return fmt.Errorf("service_token, user_id, and xiaomichatbot_ph are required for MiMo accounts")
                }
        }
        if req.Proxy != nil {
                if req.Proxy.Type != "http" && req.Proxy.Type != "https" && req.Proxy.Type != "socks5" {
                        return fmt.Errorf("proxy.type must be 'http', 'https', or 'socks5'")
                }
                if req.Proxy.Host == "" {
                        return fmt.Errorf("proxy.host is required")
                }
                if req.Proxy.Port <= 0 || req.Proxy.Port > 65535 {
                        return fmt.Errorf("proxy.port must be 1-65535")
                }
        }
        return nil
}

// saveToDB inserts or replaces an account.
func (am *AccountManager) saveToDB(a *Account) error {
        if am.db == nil {
                return nil // No DB (used in tests)
        }
        var proxyType, proxyHost, proxyUser, proxyPass sql.NullString
        var proxyPort sql.NullInt64

        if a.Proxy != nil {
                proxyType = sql.NullString{String: a.Proxy.Type, Valid: true}
                proxyHost = sql.NullString{String: a.Proxy.Host, Valid: true}
                proxyPort = sql.NullInt64{Int64: int64(a.Proxy.Port), Valid: true}
                proxyUser = sql.NullString{String: a.Proxy.Username, Valid: true}
                encPass, err := encryptPassword(a.Proxy.Password)
                if err != nil {
                        // If encryption fails (no key), store empty rather than plaintext
                        proxyPass = sql.NullString{String: "", Valid: false}
                } else {
                        proxyPass = sql.NullString{String: encPass, Valid: true}
                }
        }

        _, err := am.db.Exec(`
                INSERT OR REPLACE INTO accounts
                (id, provider, display_name, notes, zai_token, service_token,
                 user_id, xiaomichatbot_ph, proxy_type, proxy_host, proxy_port,
                 proxy_username, proxy_password, enabled, updated_at)
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
        `,
                a.ID, a.Provider, a.DisplayName, a.Notes,
                nullStr(a.ZaiToken), nullStr(a.ServiceToken),
                nullStr(a.UserID), nullStr(a.XiaomichatPH),
                proxyType, proxyHost, proxyPort, proxyUser, proxyPass,
                a.Enabled.Load(),
        )
        return err
}

func nullStr(s string) sql.NullString {
        return sql.NullString{String: s, Valid: s != ""}
}
