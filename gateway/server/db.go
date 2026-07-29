// Package server — DB initialization with WAL mode and full schema for
// accounts, model_features, aliases, rate_limits, and request_log.
package server

import (
        "database/sql"
        "fmt"
        "log"
        "os"
        "path/filepath"
        "time"

        _ "modernc.org/sqlite"
)

// InitDB opens the SQLite database, configures WAL mode and auto_vacuum,
// and creates all required tables if they don't exist.
func InitDB(path string) (*sql.DB, error) {
        // Create parent directory if it doesn't exist (fixes SQLite error 14:
        // "unable to open database file" when the data/ directory is missing).
        if dir := filepath.Dir(path); dir != "" && dir != "." {
                if err := os.MkdirAll(dir, 0755); err != nil {
                        return nil, fmt.Errorf("create db directory %q: %w", dir, err)
                }
        }

        db, err := sql.Open("sqlite", path)
        if err != nil {
                return nil, err
        }

        // SQLite is single-writer; limit to 1 connection to avoid "database is locked"
        // errors and to make :memory: databases work correctly (each connection
        // would otherwise get its own private in-memory DB).
        db.SetMaxOpenConns(1)

        // WAL for concurrency (note: :memory: databases return "memory" not "wal")
        if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
                return nil, err
        }
        if _, err := db.Exec("PRAGMA synchronous=NORMAL"); err != nil {
                return nil, err
        }
        if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
                return nil, err
        }

        // Check and enable auto_vacuum (only effective on empty DB; for existing
        // DBs we run a one-time VACUUM to activate incremental mode).
        var av int
        if err := db.QueryRow("PRAGMA auto_vacuum").Scan(&av); err != nil {
                return nil, err
        }
        if av == 0 {
                if _, err := db.Exec("PRAGMA auto_vacuum=INCREMENTAL"); err != nil {
                        log.Printf("[db] set auto_vacuum failed: %v", err)
                }
                log.Println("[db] running one-time VACUUM to activate incremental auto_vacuum...")
                if _, err := db.Exec("VACUUM"); err != nil {
                        log.Printf("[db] one-time VACUUM failed: %v (continuing)", err)
                }
        }

        schema := `
        CREATE TABLE IF NOT EXISTS accounts (
                id              TEXT PRIMARY KEY,
                provider        TEXT NOT NULL,
                display_name    TEXT NOT NULL,
                notes           TEXT,
                zai_token       TEXT,
                service_token   TEXT,
                user_id         TEXT,
                xiaomichatbot_ph TEXT,
                proxy_type      TEXT,
                proxy_host      TEXT,
                proxy_port      INTEGER,
                proxy_username  TEXT,
                proxy_password  TEXT,
                enabled         BOOLEAN DEFAULT 1,
                created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
                updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP
        );

        CREATE INDEX IF NOT EXISTS idx_accounts_provider ON accounts(provider);
        CREATE INDEX IF NOT EXISTS idx_accounts_enabled ON accounts(enabled);

        CREATE TABLE IF NOT EXISTS model_features (
                model           TEXT PRIMARY KEY,
                include_all     BOOLEAN DEFAULT 0,
                overrides_json  TEXT,
                updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP
        );

        CREATE TABLE IF NOT EXISTS model_aliases (
                alias           TEXT PRIMARY KEY,
                target_model    TEXT NOT NULL,
                created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
        );

        CREATE TABLE IF NOT EXISTS model_rate_limits (
                model           TEXT PRIMARY KEY,
                max_rpm         INTEGER,
                max_tpm         INTEGER,
                max_context     INTEGER,
                updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP
        );

        CREATE TABLE IF NOT EXISTS request_log (
                id              INTEGER PRIMARY KEY AUTOINCREMENT,
                timestamp       DATETIME DEFAULT CURRENT_TIMESTAMP,
                provider        TEXT NOT NULL,
                model           TEXT NOT NULL,
                account_id      TEXT,
                tokens_prompt   INTEGER,
                tokens_completion INTEGER,
                tokens_total    INTEGER,
                duration_ms     INTEGER,
                status_code     INTEGER,
                error_message   TEXT,
                client_ip       TEXT
        );

        CREATE INDEX IF NOT EXISTS idx_log_timestamp ON request_log(timestamp);
        CREATE INDEX IF NOT EXISTS idx_log_provider ON request_log(provider);
        CREATE INDEX IF NOT EXISTS idx_log_account ON request_log(account_id);
        `

        if _, err := db.Exec(schema); err != nil {
                return nil, err
        }

        return db, nil
}

// StartRetentionJob runs a daily job that deletes request_log entries
// older than 30 days, in batches of 10000, with a 5-minute deadline.
// After deletion, runs incremental_vacuum to reclaim space.
func StartRetentionJob(db *sql.DB) {
        ticker := time.NewTicker(24 * time.Hour)
        go func() {
                for range ticker.C {
                        runRetentionOnce(db)
                }
        }()
}

// runRetentionOnce performs one retention pass (used by StartRetentionJob
// and tests).
func runRetentionOnce(db *sql.DB) {
        deadline := time.Now().Add(5 * time.Minute)
        totalDeleted := int64(0)

        for time.Now().Before(deadline) {
                result, err := db.Exec(`
                        DELETE FROM request_log
                        WHERE id IN (
                                SELECT id FROM request_log
                                WHERE timestamp < datetime('now', '-30 days')
                                LIMIT 10000
                        )
                `)
                if err != nil {
                        log.Printf("[retention] delete failed: %v", err)
                        break
                }

                rows, _ := result.RowsAffected()
                if rows == 0 {
                        break
                }

                totalDeleted += rows
                log.Printf("[retention] deleted %d rows (total: %d)", rows, totalDeleted)
                time.Sleep(100 * time.Millisecond)
        }

        if totalDeleted > 0 {
                log.Printf("[retention] completed: %d rows deleted", totalDeleted)
        }

        if _, err := db.Exec("PRAGMA incremental_vacuum(1000)"); err != nil {
                log.Printf("[retention] incremental_vacuum failed: %v", err)
        }
}
