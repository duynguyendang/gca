package server

import (
	"context"
	"net/http"

	"github.com/duynguyendang/meb"
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

	sourceStore, srcErr := s.manager.GetSourceStore(projectID)

	// Batch-fetch all descriptions into a lookup map (eliminates N+1 per-orphan scan)
	descMap := make(map[string]string)
	if srcErr == nil {
		for fact := range sourceStore.ScanContext(c.Request.Context(), "", "okf_description", "") {
			if desc, ok := fact.Object.(string); ok {
				descMap[fact.Subject] = desc
			}
		}
	}

	type Orphan struct {
		ConceptID   string `json:"concept_id"`
		Description string `json:"description,omitempty"`
	}

	var orphans []Orphan
	for fact := range analyticalStore.ScanContext(c.Request.Context(), "", "has_smell_type", "okf_orphan_concept") {
		o := Orphan{ConceptID: fact.Subject}
		if desc, ok := descMap[fact.Subject]; ok {
			o.Description = desc
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
	scanFacts(sourceStore, c.Request.Context(), "has_role", func(fact meb.Fact) {
		if obj, ok := fact.Object.(string); ok && obj == "okf_concept" {
			conceptMap[fact.Subject] = &ConceptInfo{
				ID:   fact.Subject,
				Type: "okf_concept",
			}
		}
	})

	// Fetch titles
	scanFacts(sourceStore, c.Request.Context(), "okf_title", func(fact meb.Fact) {
		if c, ok := conceptMap[fact.Subject]; ok {
			if title, ok := fact.Object.(string); ok {
				c.Title = title
			}
		}
	})

	// Fetch descriptions
	scanFacts(sourceStore, c.Request.Context(), "okf_description", func(fact meb.Fact) {
		if c, ok := conceptMap[fact.Subject]; ok {
			if desc, ok := fact.Object.(string); ok {
				c.Description = desc
			}
		}
	})

	// Fetch types
	scanFacts(sourceStore, c.Request.Context(), "okf_concept", func(fact meb.Fact) {
		if c, ok := conceptMap[fact.Subject]; ok {
			if t, ok := fact.Object.(string); ok {
				c.Type = t
			}
		}
	})

	// Fetch tags
	scanFacts(sourceStore, c.Request.Context(), "okf_tag", func(fact meb.Fact) {
		if c, ok := conceptMap[fact.Subject]; ok {
			if tag, ok := fact.Object.(string); ok {
				c.Tags = append(c.Tags, tag)
			}
		}
	})

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

// scanFacts iterates all facts with a given predicate and calls fn for each.
func scanFacts(store *meb.MEBStore, ctx context.Context, predicate string, fn func(meb.Fact)) {
	for fact := range store.ScanContext(ctx, "", predicate, "") {
		fn(fact)
	}
}
