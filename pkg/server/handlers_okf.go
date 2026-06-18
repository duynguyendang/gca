package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/duynguyendang/gca/pkg/okf"
	"github.com/gin-gonic/gin"
)

// allowedExportDir is the directory under which export output is permitted.
// Prevents SSRF via the `out` query parameter.
const allowedExportDir = "./data/exports"

// handleOKFIngest processes an OKF bundle ingestion request.
// POST /api/v1/okf/ingest
// Body: { "project_id": "...", "bundle_dir": "/path/to/bundle" }
func (s *Server) handleOKFIngest(c *gin.Context) {
	var req struct {
		ProjectID string `json:"project_id"`
		BundleDir string `json:"bundle_dir" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}
	if req.ProjectID == "" {
		req.ProjectID = "default"
	}

	report, err := okf.Ingest(c.Request.Context(), s.manager, okfDataDir(s), okf.IngestOptions{
		ProjectID: req.ProjectID,
		BundleDir: req.BundleDir,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"concepts":      report.Concepts,
		"links":         report.Links,
		"bridges":       report.Bridges,
		"bridge_misses": report.BridgeMiss,
		"conformant":    report.Conformant,
		"duration":      report.Duration.String(),
		"errors":        report.Errors,
	})
}

// handleOKFExport exports the code graph as an OKF bundle.
// GET /api/v1/okf/export?project=...&scope=file|package|cluster&out=...
func (s *Server) handleOKFExport(c *gin.Context) {
	projectID := c.Query("project")
	if projectID == "" {
		projectID = "default"
	}
	scope := okf.ExportScope(c.DefaultQuery("scope", "file"))
	outDir := c.Query("out")

	// SSRF protection: outDir must be under the allowed export directory.
	absOut, err := filepath.Abs(outDir)
	if err != nil || !strings.HasPrefix(absOut, allowedExportDir) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("output directory must be under %s", allowedExportDir),
		})
		return
	}
	// Create allowed dir if not present
	os.MkdirAll(allowedExportDir, 0o755) //nolint:errcheck

	report, err := okf.Export(c.Request.Context(), s.manager, okf.ExportOptions{
		ProjectID:        projectID,
		OutDir:           absOut,
		Scope:            scope,
		IncludeSmells:    true,
		IncludeCitations: true,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"concepts_written": report.ConceptsWritten,
		"files_written":    report.FilesWritten,
		"duration":         report.Duration.String(),
	})
}

// handleOKFOrphans returns OKF concepts flagged by okf_orphan_concept.
// GET /api/v1/okf/orphans?project=...
func (s *Server) handleOKFOrphans(c *gin.Context) {
	projectID := c.Query("project")
	if projectID == "" {
		projectID = "default"
	}

	analyticalStore, err := s.manager.GetAnalyticalStore(projectID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Acquire source store once, not per-iteration
	sourceStore, srcErr := s.manager.GetSourceStore(projectID)

	type Orphan struct {
		ConceptID   string `json:"concept_id"`
		Description string `json:"description,omitempty"`
	}

	var orphans []Orphan
	for fact := range analyticalStore.ScanContext(c.Request.Context(), "", "has_smell_type", "okf_orphan_concept") {
		o := Orphan{ConceptID: fact.Subject}
		if srcErr == nil {
			for df := range sourceStore.ScanContext(c.Request.Context(), fact.Subject, "okf_description", "") {
				if desc, ok := df.Object.(string); ok {
					o.Description = desc
					break
				}
			}
		}
		orphans = append(orphans, o)
	}

	c.JSON(http.StatusOK, gin.H{
		"project": projectID,
		"orphans": orphans,
		"count":   len(orphans),
	})
}

// okfDataDir returns the data directory for OKF body storage.
// In production this is the same dir that StoreManager uses.
// The CLI passes it explicitly; the server extracts it from the manager's base dir.
func okfDataDir(s *Server) string {
	return "./data"
}
