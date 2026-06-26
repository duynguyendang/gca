package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

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

// handleOKFConceptsBatch returns all OKF concepts with titles in a single call.
// GET /api/v1/okf/concepts?project=...
// Eliminates N+1 query problem for frontend.
func (s *Server) handleOKFConceptsBatch(c *gin.Context) {
	projectID := c.Query("project")
	if projectID == "" {
		projectID = "default"
	}

	sourceStore, err := s.manager.GetSourceStore(projectID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	type ConceptInfo struct {
		ID          string   `json:"id"`
		Title       string   `json:"title"`
		Type        string   `json:"type"`
		Description string   `json:"description,omitempty"`
		Tags        []string `json:"tags,omitempty"`
	}

	conceptMap := make(map[string]*ConceptInfo)

	// Fetch concepts by role - scan all has_role facts and filter in Go
	// (MEB store key format may differ from what ScanContext expects)
	for fact := range sourceStore.ScanContext(c.Request.Context(), "", "has_role", "") {
		if obj, ok := fact.Object.(string); ok && obj == "okf_concept" {
			conceptMap[fact.Subject] = &ConceptInfo{
				ID:   fact.Subject,
				Type: "okf_concept",
			}
		}
	}

	// Fetch titles
	for fact := range sourceStore.ScanContext(c.Request.Context(), "", "okf_title", "") {
		if c, ok := conceptMap[fact.Subject]; ok {
			if title, ok := fact.Object.(string); ok {
				c.Title = title
			}
		}
	}

	// Fetch descriptions
	for fact := range sourceStore.ScanContext(c.Request.Context(), "", "okf_description", "") {
		if c, ok := conceptMap[fact.Subject]; ok {
			if desc, ok := fact.Object.(string); ok {
				c.Description = desc
			}
		}
	}

	// Fetch types
	for fact := range sourceStore.ScanContext(c.Request.Context(), "", "okf_concept", "") {
		if c, ok := conceptMap[fact.Subject]; ok {
			if t, ok := fact.Object.(string); ok {
				c.Type = t
			}
		}
	}

	// Fetch tags
	for fact := range sourceStore.ScanContext(c.Request.Context(), "", "okf_tag", "") {
		if c, ok := conceptMap[fact.Subject]; ok {
			if tag, ok := fact.Object.(string); ok {
				c.Tags = append(c.Tags, tag)
			}
		}
	}

	concepts := make([]ConceptInfo, 0, len(conceptMap))
	for _, c := range conceptMap {
		concepts = append(concepts, *c)
	}

	c.JSON(http.StatusOK, gin.H{
		"project":  projectID,
		"concepts": concepts,
		"count":    len(concepts),
	})
}

// handleOKFLinksBatch returns all concept-to-concept links in a single call.
// GET /api/v1/okf/links?project=...
func (s *Server) handleOKFLinksBatch(c *gin.Context) {
	projectID := c.Query("project")
	if projectID == "" {
		projectID = "default"
	}

	sourceStore, err := s.manager.GetSourceStore(projectID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	type LinkInfo struct {
		Source string `json:"source"`
		Target string `json:"target"`
	}

	var links []LinkInfo
	for fact := range sourceStore.ScanContext(c.Request.Context(), "", "okf_link", "") {
		target, ok := fact.Object.(string)
		if !ok {
			continue
		}
		links = append(links, LinkInfo{
			Source: fact.Subject,
			Target: target,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"project": projectID,
		"links":   links,
		"count":   len(links),
	})
}
