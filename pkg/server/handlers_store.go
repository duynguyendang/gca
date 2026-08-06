package server

import (
	"net/http"

	apperrors "github.com/duynguyendang/gca/pkg/common/errors"
	"github.com/duynguyendang/gca/pkg/logger"
	"github.com/gin-gonic/gin"
)

// handleStoreHealth returns operational health metrics for a project's MEB store
// via meb v0.6's DebugInfo() API.
//
// Query parameters:
//   - project: project ID whose store to inspect
//
// Response: JSON StoreHealth (num_facts, num_vectors, vector dimension/capacity,
// WAL size, circuit breaker state + metrics, last GC time, read_only).
func (s *Server) handleStoreHealth(c *gin.Context) {
	projectID := c.Query("project")
	if projectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing project parameter"})
		return
	}

	store, err := s.manager.GetStore(projectID)
	if err != nil {
		logger.Error("handleStoreHealth error", "project", projectID, "error", err)
		handleError(c, apperrors.NewAppError(http.StatusNotFound, "failed to access store", err))
		return
	}

	c.JSON(http.StatusOK, store.DebugInfo())
}