package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleFileBackbone returns the bidirectional file-level dependency graph backbone.
func (s *Server) handleFileBackbone(c *gin.Context) {
	projectID := c.Query("project")
	fileID := c.Query("id")

	if projectID == "" {
		projects, err := s.graphService.ListProjects()
		if err == nil && len(projects) > 0 {
			projectID = projects[0].ID
		}
	}

	if projectID == "" || fileID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing project or id parameter"})
		return
	}

	graph, err := s.graphService.GetFileBackbone(c.Request.Context(), projectID, fileID)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, graph)
}
