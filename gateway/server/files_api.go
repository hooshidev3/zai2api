// Package server — File upload handling.
//
// MiMo: forwards to sub-engine (UploadToXiaomi).
// GLM: returns 501 with guidance to use image_url in messages for vision models.
package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleFileUpload handles POST /v1/files.
//
// For MiMo: forwards to the MiMo sub-engine which has UploadToXiaomi.
// For GLM: returns 501 — GLM doesn't support direct file upload.
// Use image_url in messages content array for GLM vision models.
func (s *Server) handleFileUpload(c *gin.Context) {
	// Check if request is for GLM
	if c.Query("provider") == "glm" || c.GetHeader("X-Provider") == "glm" {
		c.JSON(http.StatusNotImplemented, gin.H{
			"error": gin.H{
				"type":    "glm_no_file_upload",
				"message": "GLM does not support direct file upload. For vision models (glm-4.6v, glm-4.5v), use image_url in messages content array instead.",
				"hint":    "See: https://docs.z.ai for multimodal input format",
			},
		})
		return
	}

	// MiMo: forward to sub-engine
	if s.mimoEngine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": gin.H{"type": "mimo_unavailable", "message": "MiMo not initialized"},
		})
		return
	}

	// Rewrite path for MiMo sub-engine
	c.Request.URL.Path = "/v1/files"
	s.mimoEngine.ServeHTTP(c.Writer, c.Request)
	c.Abort()
}

// isVisionModel checks if a model supports vision (image input).
func isVisionModel(model string) bool {
	visionModels := map[string]bool{
		"glm-4.5v":     true,
		"glm-4.6v":     true,
		"glm-5v":       true,
		"glm-5v-turbo": true,
		"GLM-5v-Turbo": true,
	}
	return visionModels[model]
}
