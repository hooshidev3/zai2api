// Package server — AccountManager with atomic.Bool, DTO for JSON, and
// slice order for stable round-robin.
package server

import (
        "database/sql"
        "fmt"
        "sync"
        "sync/atomic"
        "time"
)

// ProviderType identifies which upstream provider an account belongs to.
type ProviderType string

const (
        ProviderGLM  ProviderType = "glm"
        ProviderMimo ProviderType = "mimo"
)

// ProxyConfig holds optional proxy settings for an account.
type ProxyConfig struct {
        Type     string `json:"type"`
        Host     string `json:"host"`
        Port     int    `json:"port"`
        Username string `json:"username,omitempty"`
        Password string `json:"password,omitempty"`
}

// URL returns the proxy URL string (e.g. "socks5://user:pass@host:port").
func (p *ProxyConfig) URL() string {
        if p == nil {
                return ""
        }
        host := fmt.Sprintf("%s:%d", p.Host, p.Port)
        if p.Username != "" {
                return fmt.Sprintf("%s://%s:%s@%s", p.Type, p.Username, p.Password, host)
        }
        return fmt.Sprintf("%s://%s", p.Type, host)
}

// Account holds all info for a single upstream account.
// Enabled is atomic.Bool (not directly JSON-serializable; use ToDTO()).
// Stats holds runtime counters (also use ToDTO() for JSON).
type Account struct {
        ID           string       `json:"id"`
        Provider     ProviderType `json:"provider"`
        DisplayName  string       `json:"display_name"`
        Notes        string       `json:"notes"`

        // Z.AI-specific
        ZaiToken string `json:"zai_token,omitempty"`

        // MiMo-specific
        ServiceToken string `json:"service_token,omitempty"`
        UserID       string `json:"user_id,omitempty"`
        XiaomichatPH string `json:"xiaomichatbot_ph,omitempty"`

        // Shared
        Proxy     *ProxyConfig `json:"proxy,omitempty"`
        Enabled   atomic.Bool  `json:"-"`
        CreatedAt time.Time    `json:"created_at"`
        UpdatedAt time.Time    `json:"updated_at"`

        // Runtime stats
        Stats AccountStats `json:"-"`

        // Test result
        LastTestResult *TestResult  `json:"-"`
        LastTestMu     sync.RWMutex `json:"-"`
}

// AccountStats holds per-account runtime counters.
type AccountStats struct {
        ReqCount   atomic.Int64
        ErrCount   atomic.Int64
        LastUsed   time.Time
        LastErr    string
        AvgLatency atomic.Int64 // milliseconds
}

// AccountDTO is the JSON-serializable form of Account (masks tokens,
// surfaces atomic values).
type AccountDTO struct {
        ID           string       `json:"id"`
        Provider     ProviderType `json:"provider"`
        DisplayName  string       `json:"display_name"`
        Notes        string       `json:"notes"`
        ZaiTokenMask string       `json:"zai_token_mask,omitempty"`
        HasProxy     bool         `json:"has_proxy"`
        ProxyType    string       `json:"proxy_type,omitempty"`
        ProxyHost    string       `json:"proxy_host,omitempty"`
        ProxyPort    int          `json:"proxy_port,omitempty"`
        Enabled      bool         `json:"enabled"`
        CreatedAt    time.Time    `json:"created_at"`
        UpdatedAt    time.Time    `json:"updated_at"`
        ReqCount     int64        `json:"req_count"`
        ErrCount     int64        `json:"err_count"`
        AvgLatency   int64        `json:"avg_latency_ms"`
        LastUsed     time.Time    `json:"last_used"`
        LastErr      string       `json:"last_err,omitempty"`
}

// ToDTO returns a JSON-safe snapshot of the account.
func (a *Account) ToDTO() AccountDTO {
        dto := AccountDTO{
                ID:          a.ID,
                Provider:    a.Provider,
                DisplayName: a.DisplayName,
                Notes:       a.Notes,
                HasProxy:    a.Proxy != nil,
                Enabled:     a.Enabled.Load(),
                CreatedAt:   a.CreatedAt,
                UpdatedAt:   a.UpdatedAt,
                ReqCount:    a.Stats.ReqCount.Load(),
                ErrCount:    a.Stats.ErrCount.Load(),
                AvgLatency:  a.Stats.AvgLatency.Load(),
                LastUsed:    a.Stats.LastUsed,
                LastErr:     a.Stats.LastErr,
        }
        if a.ZaiToken != "" {
                dto.ZaiTokenMask = maskToken(a.ZaiToken)
        }
        if a.Proxy != nil {
                dto.ProxyType = a.Proxy.Type
                dto.ProxyHost = a.Proxy.Host
                dto.ProxyPort = a.Proxy.Port
        }
        return dto
}

// AccountManager holds all accounts (in-memory + DB-backed).
// `order` is a slice preserving insertion order for stable round-robin
// (Go map iteration order is randomized).
type AccountManager struct {
        mu       sync.RWMutex
        order    []string // stable insertion order
        accounts map[string]*Account
        strategy string
        rrIdx    atomic.Int64
        db       *sql.DB
}

// NewAccountManager loads accounts from DB and returns a manager.
// strategy: "round-robin" | "least-used" | "random"
func NewAccountManager(db *sql.DB, strategy string) *AccountManager {
        if strategy == "" {
                strategy = "round-robin"
        }
        am := &AccountManager{
                accounts: make(map[string]*Account),
                db:       db,
                strategy: strategy,
        }
        _ = am.loadFromDB()
        return am
}

func (am *AccountManager) loadFromDB() error {
        if am.db == nil {
                return nil // No DB (used in tests for validation only)
        }
        rows, err := am.db.Query(`
                SELECT id, provider, display_name, notes,
                       zai_token, service_token, user_id, xiaomichatbot_ph,
                       proxy_type, proxy_host, proxy_port, proxy_username, proxy_password,
                       enabled
                FROM accounts
                ORDER BY created_at ASC
        `)
        if err != nil {
                return err
        }
        defer rows.Close()

        for rows.Next() {
                var a Account
                var proxyType, proxyHost, proxyUser, proxyPass sql.NullString
                var proxyPort sql.NullInt64
                var zaiToken, serviceToken, userID, xiaomiPH, notes sql.NullString
                var enabled bool

                err := rows.Scan(
                        &a.ID, &a.Provider, &a.DisplayName, &notes,
                        &zaiToken, &serviceToken, &userID, &xiaomiPH,
                        &proxyType, &proxyHost, &proxyPort, &proxyUser, &proxyPass,
                        &enabled,
                )
                if err != nil {
                        continue
                }

                a.ZaiToken = zaiToken.String
                a.ServiceToken = serviceToken.String
                a.UserID = userID.String
                a.XiaomichatPH = xiaomiPH.String
                a.Notes = notes.String
                a.Enabled.Store(enabled)

                if proxyType.Valid {
                        pass, _ := decryptPassword(proxyPass.String)
                        a.Proxy = &ProxyConfig{
                                Type:     proxyType.String,
                                Host:     proxyHost.String,
                                Port:     int(proxyPort.Int64),
                                Username: proxyUser.String,
                                Password: pass,
                        }
                }

                am.accounts[a.ID] = &a
                am.order = append(am.order, a.ID)
        }
        return nil
}

// Add adds an account (used by tests; production uses Create which validates).
// If Enabled is not set (zero value), it defaults to true.
func (am *AccountManager) Add(a *Account) {
        am.mu.Lock()
        defer am.mu.Unlock()
        // Default to enabled if not explicitly set
        if !a.Enabled.Load() {
                a.Enabled.Store(true)
        }
        if _, exists := am.accounts[a.ID]; !exists {
                am.order = append(am.order, a.ID)
        }
        am.accounts[a.ID] = a
}

// Next returns the next active account for the given provider.
// Strategy: round-robin | least-used | random.
func (am *AccountManager) Next(provider ProviderType) (*Account, error) {
        am.mu.RLock()
        defer am.mu.RUnlock()

        var active []*Account
        for _, id := range am.order {
                a := am.accounts[id]
                if a.Provider == provider && a.Enabled.Load() {
                        active = append(active, a)
                }
        }

        if len(active) == 0 {
                return nil, fmt.Errorf("no active %s accounts", provider)
        }

        switch am.strategy {
        case "round-robin":
                idx := int(am.rrIdx.Add(1)-1) % len(active)
                return active[idx], nil
        case "least-used":
                best := active[0]
                for _, a := range active[1:] {
                        if a.Stats.ReqCount.Load() < best.Stats.ReqCount.Load() {
                                best = a
                        }
                }
                return best, nil
        default: // random
                return active[time.Now().UnixNano()%int64(len(active))], nil
        }
}

// MarkFailed increments error count and auto-disables after 5 errors.
func (am *AccountManager) MarkFailed(id string, err error) {
        am.mu.RLock()
        defer am.mu.RUnlock()

        if a, ok := am.accounts[id]; ok {
                a.Stats.ErrCount.Add(1)
                a.Stats.LastErr = err.Error()

                if a.Stats.ErrCount.Load() > 5 {
                        a.Enabled.Store(false)
                        _ = am.saveToDB(a)
                }
        }
}

// MarkSuccess increments request count and updates average latency.
func (am *AccountManager) MarkSuccess(id string, latency time.Duration) {
        am.mu.RLock()
        defer am.mu.RUnlock()

        if a, ok := am.accounts[id]; ok {
                a.Stats.ReqCount.Add(1)
                a.Stats.LastUsed = time.Now()

                count := a.Stats.ReqCount.Load()
                prevAvg := a.Stats.AvgLatency.Load()
                newAvg := (prevAvg*(count-1) + latency.Milliseconds()) / count
                a.Stats.AvgLatency.Store(newAvg)
        }
}

// List returns all accounts (optionally filtered by provider).
func (am *AccountManager) List(provider ProviderType) []*Account {
        am.mu.RLock()
        defer am.mu.RUnlock()

        var out []*Account
        for _, id := range am.order {
                a := am.accounts[id]
                if provider == "" || a.Provider == provider {
                        out = append(out, a)
                }
        }
        return out
}

// Get returns an account by ID.
func (am *AccountManager) Get(id string) (*Account, bool) {
        am.mu.RLock()
        defer am.mu.RUnlock()
        a, ok := am.accounts[id]
        return a, ok
}

// maskToken returns a masked version of a token for safe display.
func maskToken(t string) string {
        if len(t) < 12 {
                return "***"
        }
        return t[:6] + "..." + t[len(t)-4:]
}
