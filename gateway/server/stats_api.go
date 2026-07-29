// Package server — Detailed stats API with filtering and CSV export.
package server

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// ModelStats holds aggregated stats for one model.
type ModelStats struct {
	Model      string  `json:"model"`
	Provider   string  `json:"provider"`
	Requests   int64   `json:"requests"`
	Tokens     int64   `json:"tokens"`
	AvgLatency int64   `json:"avg_latency_ms"`
	ErrorRate  float64 `json:"error_rate"`
}

// handleDetailedStats returns per-model stats from request_log.
// Uses COALESCE to handle NULL model/provider columns.
func (s *Server) handleDetailedStats(c *gin.Context) {
	if s.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"type": "db_unavailable", "message": "database not initialized"}})
		return
	}

	provider := c.Query("provider")
	rangeStr := c.DefaultQuery("range", "24h")
	since := parseTimeRange(rangeStr)

	// COALESCE ensures NULL model/provider are shown as 'unknown' instead of being skipped
	query := `
		SELECT COALESCE(model, 'unknown') as model,
		       COALESCE(provider, 'unknown') as provider,
		       COUNT(*) as requests,
		       COALESCE(SUM(tokens_total), 0) as tokens,
		       COALESCE(CAST(AVG(duration_ms) AS INTEGER), 0) as avg_latency,
		       COALESCE(SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END) * 100.0 / COUNT(*), 0) as error_rate
		FROM request_log
		WHERE timestamp > ?
	`
	args := []interface{}{since}

	if provider != "" {
		query += " AND provider = ?"
		args = append(args, provider)
	}

	query += " GROUP BY model, provider ORDER BY requests DESC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var stats []ModelStats
	for rows.Next() {
		var ms ModelStats
		if err := rows.Scan(&ms.Model, &ms.Provider, &ms.Requests,
			&ms.Tokens, &ms.AvgLatency, &ms.ErrorRate); err != nil {
			continue
		}
		stats = append(stats, ms)
	}

	c.JSON(http.StatusOK, gin.H{
		"range":     rangeStr,
		"provider":  provider,
		"models":    stats,
		"generated": time.Now().UTC().Format(time.RFC3339),
	})
}

// handleExportStatsCSV exports request_log as CSV.
// Uses sql.NullString for nullable columns (account_id, error_message).
// Sanitizes rangeStr in the filename to prevent header injection.
func (s *Server) handleExportStatsCSV(c *gin.Context) {
	if s.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not initialized"})
		return
	}

	provider := c.Query("provider")
	rangeStr := c.DefaultQuery("range", "24h")
	since := parseTimeRange(rangeStr)

	// Sanitize rangeStr for filename (only allow known values)
	safeRange := "24h"
	switch rangeStr {
	case "1h", "7d", "24h":
		safeRange = rangeStr
	}

	query := `
		SELECT timestamp, provider, model, account_id,
		       tokens_prompt, tokens_completion, tokens_total,
		       duration_ms, status_code, error_message
		FROM request_log
		WHERE timestamp > ?
	`
	args := []interface{}{since}
	if provider != "" {
		query += " AND provider = ?"
		args = append(args, provider)
	}
	query += " ORDER BY timestamp DESC LIMIT 10000"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition",
		fmt.Sprintf("attachment; filename=stats_%s_%s.csv", safeRange, time.Now().Format("20060102_150405")))

	writer := csv.NewWriter(c.Writer)
	defer writer.Flush()

	writer.Write([]string{
		"timestamp", "provider", "model", "account_id",
		"tokens_prompt", "tokens_completion", "tokens_total",
		"duration_ms", "status_code", "error_message",
	})

	for rows.Next() {
		var ts, prov, model string
		var accountID, errMsg sql.NullString // nullable columns
		var tokPrompt, tokComp, tokTotal, duration, statusCode int
		if err := rows.Scan(&ts, &prov, &model, &accountID,
			&tokPrompt, &tokComp, &tokTotal, &duration, &statusCode, &errMsg); err != nil {
			continue // skip corrupt rows
		}
		writer.Write([]string{
			ts, prov, model, accountID.String,
			strconv.Itoa(tokPrompt), strconv.Itoa(tokComp), strconv.Itoa(tokTotal),
			strconv.Itoa(duration), strconv.Itoa(statusCode), errMsg.String,
		})
	}
}

// parseTimeRange converts "1h", "24h", "7d" to a time.Time.
func parseTimeRange(rangeStr string) time.Time {
	switch rangeStr {
	case "1h":
		return time.Now().Add(-1 * time.Hour)
	case "7d":
		return time.Now().Add(-7 * 24 * time.Hour)
	default: // 24h
		return time.Now().Add(-24 * time.Hour)
	}
}
