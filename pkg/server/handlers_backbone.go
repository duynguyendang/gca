package server

import (
	"net/http"

	"github.com/duynguyendang/gca/pkg/logger"
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

	autocluster := c.Query("nocluster") != "true"

	graph, err := s.graphService.GetFileBackbone(c.Request.Context(), projectID, fileID)
	if err != nil {
		handleError(c, err)
		return
	}

	// Auto-cluster if too many nodes (>500)
	if autocluster && len(graph.Nodes) > 500 {
		logger.Debug("Auto-Clustering Backbone clustering", "nodes", len(graph.Nodes))
		clustered, clusterErr := s.graphService.ClusterGraphData(graph)
		if clusterErr == nil && len(clustered.Nodes) > 0 {
			logger.Debug("Auto-Clustering Success", "clusterNodes", len(clustered.Nodes))
			c.JSON(http.StatusOK, clustered)
			return
		}
		logger.Warn("Auto-Clustering Failed", "error", clusterErr)
	}

	c.JSON(http.StatusOK, graph)
}
