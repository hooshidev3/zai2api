package server

import (
        "fmt"
        "os"
        "strings"
        "testing"
)

func TestInitDB(t *testing.T) {
        db, err := InitDB(":memory:")
        if err != nil {
                t.Fatalf("InitDB failed: %v", err)
        }
        defer db.Close()

        // Verify tables exist (WAL mode doesn't work with :memory:, so we
        // check tables instead of journal_mode here)
        for _, table := range []string{"accounts", "model_features", "model_aliases", "model_rate_limits", "request_log"} {
                var count int
                if err := db.QueryRow("SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
                        t.Errorf("%s table not created: %v", table, err)
                }
        }
}

func TestInitDBWALMode(t *testing.T) {
        // WAL mode only works with file-based databases, not :memory:
        tmpFile := t.TempDir() + "/test.db"
        db, err := InitDB(tmpFile)
        if err != nil {
                t.Fatalf("InitDB failed: %v", err)
        }
        defer db.Close()

        var mode string
        if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
                t.Fatal(err)
        }
        if mode != "wal" {
                t.Errorf("Expected WAL mode for file DB, got %s", mode)
        }
}

func TestAccountManagerRoundRobin(t *testing.T) {
        db, _ := InitDB(":memory:")
        defer db.Close()

        am := NewAccountManager(db, "round-robin")

        am.Add(&Account{ID: "zai-1", Provider: ProviderGLM, ZaiToken: "token1"})
        am.Add(&Account{ID: "zai-2", Provider: ProviderGLM, ZaiToken: "token2"})
        am.Add(&Account{ID: "mimo-1", Provider: ProviderMimo, ServiceToken: "stoken1"})

        a1, _ := am.Next(ProviderGLM)
        a2, _ := am.Next(ProviderGLM)
        a3, _ := am.Next(ProviderGLM)

        if a1.ID != "zai-1" || a2.ID != "zai-2" || a3.ID != "zai-1" {
                t.Errorf("Round-robin failed: %s, %s, %s", a1.ID, a2.ID, a3.ID)
        }

        m1, _ := am.Next(ProviderMimo)
        if m1.ID != "mimo-1" {
                t.Errorf("Expected mimo-1, got %s", m1.ID)
        }
}

func TestAccountManagerLeastUsed(t *testing.T) {
        db, _ := InitDB(":memory:")
        defer db.Close()

        am := NewAccountManager(db, "least-used")
        am.Add(&Account{ID: "a1", Provider: ProviderGLM, ZaiToken: "t1"})
        am.Add(&Account{ID: "a2", Provider: ProviderGLM, ZaiToken: "t2"})

        // a1 used 5 times, a2 used 2 times
        for i := 0; i < 5; i++ {
                am.MarkSuccess("a1", 0)
        }
        for i := 0; i < 2; i++ {
                am.MarkSuccess("a2", 0)
        }

        next, _ := am.Next(ProviderGLM)
        if next.ID != "a2" {
                t.Errorf("Least-used should pick a2 (2 reqs), got %s (%d reqs)",
                        next.ID, next.Stats.ReqCount.Load())
        }
}

func TestAccountManagerMarkFailedAutoDisable(t *testing.T) {
        db, _ := InitDB(":memory:")
        defer db.Close()

        // Set encryption key so saveToDB works
        os.Setenv("PROXY_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
        defer os.Unsetenv("PROXY_ENCRYPTION_KEY")

        am := NewAccountManager(db, "round-robin")
        am.Add(&Account{ID: "a1", Provider: ProviderGLM, ZaiToken: "t1"})

        // 5 errors should NOT disable (threshold is >5)
        for i := 0; i < 5; i++ {
                am.MarkFailed("a1", fmt.Errorf("err %d", i))
        }
        a, _ := am.Get("a1")
        if !a.Enabled.Load() {
                t.Error("Account should still be enabled after 5 errors")
        }

        // 6th error should disable
        am.MarkFailed("a1", fmt.Errorf("err 6"))
        a, _ = am.Get("a1")
        if a.Enabled.Load() {
                t.Error("Account should be disabled after 6 errors")
        }
}

func TestAccountDTO(t *testing.T) {
        a := &Account{
                ID:       "zai-1",
                Provider: ProviderGLM,
                ZaiToken: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
        }
        a.Enabled.Store(true)

        dto := a.ToDTO()
        if dto.ZaiTokenMask != "eyJhbG...VCJ9" {
                t.Errorf("Token mask failed: %s", dto.ZaiTokenMask)
        }
        if !dto.Enabled {
                t.Error("Enabled should be true")
        }
        if dto.HasProxy {
                t.Error("HasProxy should be false")
        }
}

func TestAccountCRUD(t *testing.T) {
        os.Setenv("PROXY_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
        defer os.Unsetenv("PROXY_ENCRYPTION_KEY")

        db, _ := InitDB(":memory:")
        defer db.Close()

        am := NewAccountManager(db, "round-robin")

        // Create
        acct, err := am.Create(CreateAccountRequest{
                ID:       "zai-1",
                Provider: ProviderGLM,
                ZaiToken: "token123",
        })
        if err != nil {
                t.Fatalf("Create failed: %v", err)
        }
        if acct.ID != "zai-1" {
                t.Errorf("Expected zai-1, got %s", acct.ID)
        }

        // Read (List)
        list := am.List(ProviderGLM)
        if len(list) != 1 {
                t.Errorf("Expected 1 account, got %d", len(list))
        }

        // Update
        _, err = am.Update("zai-1", UpdateAccountRequest{DisplayName: "Updated"})
        if err != nil {
                t.Fatalf("Update failed: %v", err)
        }
        a, _ := am.Get("zai-1")
        if a.DisplayName != "Updated" {
                t.Errorf("Expected 'Updated', got %s", a.DisplayName)
        }

        // Toggle
        err = am.Toggle("zai-1", false)
        if err != nil {
                t.Fatalf("Toggle failed: %v", err)
        }
        a, _ = am.Get("zai-1")
        if a.Enabled.Load() {
                t.Error("Account should be disabled")
        }

        // Delete
        err = am.Delete("zai-1")
        if err != nil {
                t.Fatalf("Delete failed: %v", err)
        }
        if len(am.List("")) != 0 {
                t.Error("Account should be deleted")
        }
}

func TestValidateAccount(t *testing.T) {
        am := NewAccountManager(nil, "")

        // Missing ID
        err := am.validate(CreateAccountRequest{Provider: ProviderGLM, ZaiToken: "t"})
        if err == nil || !strings.Contains(err.Error(), "id is required") {
                t.Errorf("Expected 'id is required', got %v", err)
        }

        // Invalid provider
        err = am.validate(CreateAccountRequest{ID: "x", Provider: "invalid"})
        if err == nil || !strings.Contains(err.Error(), "provider must be") {
                t.Errorf("Expected provider error, got %v", err)
        }

        // GLM without token
        err = am.validate(CreateAccountRequest{ID: "x", Provider: ProviderGLM})
        if err == nil || !strings.Contains(err.Error(), "zai_token is required") {
                t.Errorf("Expected zai_token error, got %v", err)
        }

        // MiMo without all fields
        err = am.validate(CreateAccountRequest{ID: "x", Provider: ProviderMimo, ServiceToken: "t"})
        if err == nil || !strings.Contains(err.Error(), "service_token, user_id") {
                t.Errorf("Expected MiMo field error, got %v", err)
        }

        // Invalid proxy type
        err = am.validate(CreateAccountRequest{
                ID: "x", Provider: ProviderGLM, ZaiToken: "t",
                Proxy: &ProxyConfig{Type: "invalid", Host: "h", Port: 8080},
        })
        if err == nil || !strings.Contains(err.Error(), "proxy.type") {
                t.Errorf("Expected proxy.type error, got %v", err)
        }
}
