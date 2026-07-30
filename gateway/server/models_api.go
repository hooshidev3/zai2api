// Package server — Models API handlers (aggregated list + feature config + refresh).
package server

import (
        "encoding/json"
        "net/http"
        "time"

        "github.com/gin-gonic/gin"
)

// handleAggregatedModels returns the combined GLM + MiMo model list
// (same as /v1/models but under /api/v1 for the dashboard).
//
// GLM models are fetched live from the Z.AI API (via FetchModels, cached
// 5 min in the GLM provider). If the fetch returns empty (e.g., Z.AI
// unreachable or GLM provider not initialized), a static fallback list
// is used so the dashboard always shows something.
//
// MiMo models are fetched from the MiMo sub-engine via
// forwardToMiMoAndCapture (non-streaming GET only).
func (s *Server) handleAggregatedModels(c *gin.Context) {
        type modelItem struct {
                ID           string                 `json:"id"`
                Object       string                 `json:"object"`
                OwnedBy      string                 `json:"owned_by"`
                Provider     string                 `json:"_provider"`
                Name         string                 `json:"name,omitempty"`
                Description  string                 `json:"description,omitempty"`
                Capabilities map[string]interface{} `json:"capabilities,omitempty"`
        }

        var models []modelItem

        // GLM models — fetch from provider with details, fallback to static list
        if s.glm != nil {
                detailed := s.glm.FetchModelsWithDetails()
                if len(detailed) == 0 {
                        // Static fallback — ensures dashboard always shows GLM models
                        for _, id := range []string{"glm-5.2", "GLM-5.1", "glm-4.7", "GLM-5-Turbo", "GLM-5v-Turbo"} {
                                models = append(models, modelItem{
                                        ID: id, Object: "model", OwnedBy: "zai", Provider: "glm", Name: id,
                                })
                        }
                } else {
                        for _, m := range detailed {
                                models = append(models, modelItem{
                                        ID:           m.ID,
                                        Object:       "model",
                                        OwnedBy:      "zai",
                                        Provider:     "glm",
                                        Name:         m.Name,
                                        Description:  m.Description,
                                        Capabilities: m.Capabilities,
                                })
                        }
                }
        } else {
                // GLM not initialized — show fallback so the UI is not empty
                for _, id := range []string{"glm-5.2", "GLM-5.1", "glm-4.7", "GLM-5-Turbo", "GLM-5v-Turbo"} {
                        models = append(models, modelItem{
                                ID: id, Object: "model", OwnedBy: "zai", Provider: "glm", Name: id,
                        })
                }
        }

        // MiMo models from sub-engine
        mimoResp, err := s.forwardToMiMoAndCapture(c, "/v1/models")
        if err == nil && mimoResp != nil {
                var mimoList struct {
                        Data []struct {
                                ID      string `json:"id"`
                                OwnedBy string `json:"owned_by"`
                        } `json:"data"`
                }
                if json.Unmarshal(mimoResp, &mimoList) == nil {
                        for _, m := range mimoList.Data {
                                models = append(models, modelItem{
                                        ID: m.ID, Object: "model", OwnedBy: m.OwnedBy, Provider: "mimo", Name: m.ID,
                                })
                        }
                }
        }

        c.JSON(http.StatusOK, gin.H{
                "object":     "list",
                "data":       models,
                "generated_at": nowISO(),
        })
}

// handleRefreshModels invalidates the GLM model cache and re-fetches.
// Called by the dashboard "Refresh" button or automatically after a new
// GLM account is added.
func (s *Server) handleRefreshModels(c *gin.Context) {
        if s.glm == nil {
                c.JSON(http.StatusServiceUnavailable, gin.H{
                        "error": gin.H{"type": "glm_unavailable", "message": "GLM provider not initialized"},
                })
                return
        }
        ids := s.glm.RefreshModels()
        c.JSON(http.StatusOK, gin.H{
                "refreshed":   true,
                "model_count": len(ids),
                "models":      ids,
                "refreshed_at": nowISO(),
        })
}

// nowISO returns the current time in RFC3339 format.
func nowISO() string {
        return time.Now().UTC().Format(time.RFC3339)
}

// handleUpdateModelFeatures updates per-model feature state for a GLM model.
// Stores in DB (model_features table) and updates GLM provider memory.
func (s *Server) handleUpdateModelFeatures(c *gin.Context) {
        model := c.Param("id")

        var req struct {
                IncludeAll bool                   `json:"include_all"`
                Overrides  map[string]interface{} `json:"overrides"`
        }
        if err := c.ShouldBindJSON(&req); err != nil {
                c.JSON(http.StatusBadRequest, gin.H{
                        "error": gin.H{"type": "invalid_request", "message": err.Error()},
                })
                return
        }

        // 1. Update GLM provider memory (if available)
        if s.glm != nil {
                s.glm.SetFeatureState(model, req.IncludeAll, req.Overrides)
        }

        // 2. Persist to DB (model_features table)
        if s.db != nil {
                overridesJSON, _ := json.Marshal(req.Overrides)
                _, err := s.db.Exec(`
                        INSERT OR REPLACE INTO model_features (model, include_all, overrides_json, updated_at)
                        VALUES (?, ?, ?, CURRENT_TIMESTAMP)
                `, model, req.IncludeAll, string(overridesJSON))
                if err != nil {
                        c.JSON(http.StatusInternalServerError, gin.H{
                                "error": gin.H{
                                        "type":    "db_error",
                                        "message": "Updated in memory but failed to persist: " + err.Error(),
                                },
                        })
                        return
                }
        }

        c.JSON(http.StatusOK, gin.H{
                "model":       model,
                "include_all": req.IncludeAll,
                "overrides":   req.Overrides,
        })
}
