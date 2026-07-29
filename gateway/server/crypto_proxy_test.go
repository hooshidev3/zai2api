package server

import (
        "net/http"
        "os"
        "strings"
        "testing"
)

func TestEncryptDecryptPassword(t *testing.T) {
        os.Setenv("PROXY_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
        defer os.Unsetenv("PROXY_ENCRYPTION_KEY")

        plaintext := "my-secret-password"
        encrypted, err := encryptPassword(plaintext)
        if err != nil {
                t.Fatalf("Encrypt failed: %v", err)
        }
        if !strings.HasPrefix(encrypted, "enc:") {
                t.Errorf("Expected enc: prefix, got %s", encrypted)
        }
        if encrypted == "enc:"+plaintext {
                t.Error("Encryption did not actually encrypt")
        }

        decrypted, err := decryptPassword(encrypted)
        if err != nil {
                t.Fatalf("Decrypt failed: %v", err)
        }
        if decrypted != plaintext {
                t.Errorf("Expected %s, got %s", plaintext, decrypted)
        }
}

func TestEncryptPasswordNoKey(t *testing.T) {
        os.Unsetenv("PROXY_ENCRYPTION_KEY")
        _, err := encryptPassword("test")
        if err == nil {
                t.Error("Expected error when PROXY_ENCRYPTION_KEY not set")
        }
}

func TestEncryptPasswordInvalidKey(t *testing.T) {
        os.Setenv("PROXY_ENCRYPTION_KEY", "not-valid-hex")
        defer os.Unsetenv("PROXY_ENCRYPTION_KEY")
        _, err := encryptPassword("test")
        if err == nil {
                t.Error("Expected error for invalid hex key")
        }
}

func TestEncryptPasswordShortKey(t *testing.T) {
        os.Setenv("PROXY_ENCRYPTION_KEY", "0123456789abcdef") // 16 chars = 8 bytes
        defer os.Unsetenv("PROXY_ENCRYPTION_KEY")
        _, err := encryptPassword("test")
        if err == nil {
                t.Error("Expected error for short key (not 32 bytes)")
        }
}

func TestDecryptPasswordPlaintextPassthrough(t *testing.T) {
        result, err := decryptPassword("plaintext-password")
        if err != nil {
                t.Fatalf("Unexpected error: %v", err)
        }
        if result != "plaintext-password" {
                t.Errorf("Expected passthrough, got %s", result)
        }
}

func TestProxyConfigURL(t *testing.T) {
        cfg := &ProxyConfig{
                Type:     "socks5",
                Host:     "10.0.0.1",
                Port:     1080,
                Username: "user",
                Password: "pass",
        }
        expected := "socks5://user:pass@10.0.0.1:1080"
        if cfg.URL() != expected {
                t.Errorf("Expected %s, got %s", expected, cfg.URL())
        }

        cfg2 := &ProxyConfig{Type: "http", Host: "proxy.local", Port: 8080}
        expected2 := "http://proxy.local:8080"
        if cfg2.URL() != expected2 {
                t.Errorf("Expected %s, got %s", expected2, cfg2.URL())
        }

        var nilCfg *ProxyConfig
        if nilCfg.URL() != "" {
                t.Error("Nil config should return empty URL")
        }
}

func TestGetHTTPClientCache(t *testing.T) {
        c1 := GetHTTPClient("test-account-1", nil, 0)
        c2 := GetHTTPClient("test-account-1", nil, 0)
        if c1 != c2 {
                t.Error("Cache should return same client for same account")
        }

        c3 := GetHTTPClient("test-account-2", nil, 0)
        if c1 == c3 {
                t.Error("Different accounts should get different clients")
        }
}

func TestInvalidateClient(t *testing.T) {
        c1 := GetHTTPClient("invalidate-test", nil, 0)
        InvalidateClient("invalidate-test")
        c2 := GetHTTPClient("invalidate-test", nil, 0)
        if c1 == c2 {
                t.Error("After invalidation, should get new client")
        }
}

func TestResolveClient(t *testing.T) {
        // ResolveClient(nil) should return http.DefaultClient
        result := ResolveClient(nil)
        if result == nil {
                t.Error("ResolveClient(nil) should not return nil")
        }
        if result != http.DefaultClient {
                t.Error("ResolveClient(nil) should return http.DefaultClient")
        }

        // Non-nil client returned as-is
        custom := &http.Client{}
        if ResolveClient(custom) != custom {
                t.Error("ResolveClient should return same client if non-nil")
        }
}

func TestRetentionJobOnce(t *testing.T) {
        db, _ := InitDB(":memory:")
        defer db.Close()

        // Insert an old record (31 days ago)
        _, err := db.Exec(`INSERT INTO request_log (timestamp, provider, model)
                VALUES (datetime('now', '-31 days'), 'glm', 'glm-5')`)
        if err != nil {
                t.Fatal(err)
        }

        // Insert a recent record
        _, err = db.Exec(`INSERT INTO request_log (provider, model) VALUES ('mimo', 'mimo-7b')`)
        if err != nil {
                t.Fatal(err)
        }

        // Run retention
        runRetentionOnce(db)

        // Old record should be deleted, recent should remain
        var count int
        db.QueryRow("SELECT COUNT(*) FROM request_log").Scan(&count)
        if count != 1 {
                t.Errorf("Expected 1 remaining record, got %d", count)
        }
}

