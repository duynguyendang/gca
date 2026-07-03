package server

import (
	"fmt"
	"net/http"

	apperrors "github.com/duynguyendang/gca/pkg/common/errors"
	"github.com/gin-gonic/gin"
)

func extractProjectID(c *gin.Context) (string, bool) {
	projectID := c.Query("project")
	if err := ValidateProjectID(projectID); err != nil {
		handleError(c, apperrors.NewAppError(http.StatusBadRequest, "project ID is required or invalid", err))
		return "", false
	}
	return projectID, true
}

func extractSymbolID(c *gin.Context, key string) (string, bool) {
	id := c.Query(key)
	if err := ValidateSymbolID(id); err != nil {
		handleError(c, apperrors.NewAppError(http.StatusBadRequest, fmt.Sprintf("%s is required or invalid", key), err))
		return "", false
	}
	return id, true
}

func setDefaultProject(c *gin.Context, s *Server) (string, bool) {
	projectID := c.Query("project")
	if projectID == "" {
		projects, err := s.graphService.ListProjects()
		if err == nil && len(projects) > 0 {
			projectID = projects[0].ID
		}
	}
	if projectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing project parameter"})
		return "", false
	}
	return projectID, true
}

func extractProjectIDFromBody(c *gin.Context, projectID string) bool {
	if err := ValidateProjectID(projectID); err != nil {
		handleError(c, apperrors.NewAppError(http.StatusBadRequest, "invalid project ID", err))
		return false
	}
	return true
}

func setDefaultProjectWithValidation(c *gin.Context, s *Server) (string, bool) {
	projectID, ok := setDefaultProject(c, s)
	if !ok {
		return "", false
	}
	if err := ValidateProjectID(projectID); err != nil {
		handleError(c, apperrors.NewAppError(http.StatusBadRequest, "invalid project ID", err))
		return "", false
	}
	return projectID, true
}
