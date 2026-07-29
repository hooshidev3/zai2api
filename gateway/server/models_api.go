// Package server — Models API handlers (aggregated list + feature config).
package server

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleAggregatedModels returns the combined GLM + MiMo model list
// (same as /v1/models but under /api/v1 for the dashboard).
func (s *Server) handleAggregatedModels(c *gin.Context) {
	type modelItem struct {
		ID       string `json:"id"`
		Object   string `json:"object"`
		OwnedBy  string `json:"owned_by"`
		Provider string `json:"_provider"`
	}

	var models []modelItem

	// GLM models — fetch from provider
	if s.glm != nil {
		glmModels := s.glm.FetchModels()
		if len(glmModels) == 0 {
			glmModels = []string{"glm-5", "glm-5.1", "glm-4.7", "GLM-5-Turbo", "GLM-5v-Turbo"}
		}
		for _, m := range glmModels {
			models = append(models, modelItem{
				ID: m, Object: "model", OwnedBy: "zai", Provider: "glm",
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
					ID: m.ID, Object: "model", OwnedBy: m.OwnedBy, Provider: "mimo",
				})
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   models,
	})
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
