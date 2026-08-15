package server

import (
	"net/http"
	"sort"
	"strings"

	apperrors "github.com/duynguyendang/gca/pkg/common/errors"
	"github.com/duynguyendang/gca/pkg/compliance"
	"github.com/gin-gonic/gin"
)

// handleComplianceVulnerabilities returns matched vulnerabilities (F4).
//
//	GET /api/v1/compliance/vulnerabilities?project=X&severity=high,medium
func (s *Server) handleComplianceVulnerabilities(c *gin.Context) {
	projectID := c.Query("project")
	if projectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing project parameter"})
		return
	}
	severityFilter := map[string]bool{}
	for _, sv := range strings.Split(c.Query("severity"), ",") {
		if sv = strings.TrimSpace(sv); sv != "" {
			severityFilter[strings.ToLower(sv)] = true
		}
	}

	analytical, err := s.manager.GetAnalyticalStore(projectID)
	if err != nil {
		handleError(c, apperrors.NewAppError(http.StatusInternalServerError, "failed to get store", err))
		return
	}

	// (Package -> AdvisoryID) from has_vulnerability, then enrich with severity/summary.
	type vuln struct {
		Package      string `json:"package"`
		AdvisoryID   string `json:"advisory_id"`
		Severity     string `json:"severity"`
		Summary      string `json:"summary"`
		SnapshotDate string `json:"snapshot_date"`
	}

	severityByID := map[string]string{}
	for fact := range analytical.ScanContext(c.Request.Context(), "", compliance.PredicateVulnSeverity, "") {
		if id := fact.Subject; id != "" {
			if sev, ok := fact.Object.(string); ok {
				severityByID[id] = sev
			}
		}
	}
	summaryByID := map[string]string{}
	for fact := range analytical.ScanContext(c.Request.Context(), "", compliance.PredicateVulnSummary, "") {
		if id := fact.Subject; id != "" {
			if sum, ok := fact.Object.(string); ok {
				summaryByID[id] = sum
			}
		}
	}

	snapDate := ""
	if snap, err := compliance.LoadSnapshot(compliance.DefaultSnapshotPath); err == nil {
		snapDate = snap.SnapshotDate()
	}

	var out []vuln
	for fact := range analytical.ScanContext(c.Request.Context(), "", compliance.PredicateHasVulnerability, "") {
		if fact.Subject == "" {
			continue
		}
		advID, ok := fact.Object.(string)
		if !ok || advID == "" {
			continue
		}
		sev := severityByID[advID]
		if len(severityFilter) > 0 && !severityFilter[strings.ToLower(sev)] {
			continue
		}
		out = append(out, vuln{
			Package:      fact.Subject,
			AdvisoryID:   advID,
			Severity:     sev,
			Summary:      summaryByID[advID],
			SnapshotDate: snapDate,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Package != out[j].Package {
			return out[i].Package < out[j].Package
		}
		return out[i].AdvisoryID < out[j].AdvisoryID
	})

	c.JSON(http.StatusOK, gin.H{
		"project_id":        projectID,
		"vulnerabilities":   out,
		"snapshot_date":     snapDate,
		"total":             len(out),
	})
}

// handleComplianceSBOM returns the project's dependency inventory (F4).
//
//	GET /api/v1/compliance/sbom?project=X&format=json|cyclonedx
func (s *Server) handleComplianceSBOM(c *gin.Context) {
	projectID := c.Query("project")
	if projectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing project parameter"})
		return
	}

	source, err := s.manager.GetSourceStore(projectID)
	if err != nil {
		handleError(c, apperrors.NewAppError(http.StatusInternalServerError, "failed to get store", err))
		return
	}

	inv, err := compliance.CollectInventory(c.Request.Context(), source)
	if err != nil {
		handleError(c, apperrors.NewAppError(http.StatusInternalServerError, "failed to collect inventory", err))
		return
	}

	format := c.DefaultQuery("format", "json")
	switch format {
	case "cyclonedx":
		c.JSON(http.StatusOK, toCycloneDX(projectID, inv))
	default:
		c.JSON(http.StatusOK, gin.H{
			"project_id":     projectID,
			"format":         "json",
			"package_count":  inv.PackageCount,
			"dependencies":   inv.Dependencies,
		})
	}
}

// toCycloneDX renders the inventory as a minimal CycloneDX 1.5 JSON document.
func toCycloneDX(projectID string, inv *compliance.Inventory) gin.H {
	components := make([]gin.H, 0, inv.PackageCount)
	for _, dep := range inv.Dependencies {
		components = append(components, gin.H{
			"type":    "library",
			"name":    dep.Name,
			"version": dep.Version,
			"purl":    "pkg:generic/" + dep.Name,
		})
	}
	return gin.H{
		"bomFormat":   "CycloneDX",
		"specVersion": "1.5",
		"version":     1,
		"metadata": gin.H{
			"component": gin.H{"type": "application", "name": projectID},
		},
		"components": components,
	}
}