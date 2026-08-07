package okf

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/duynguyendang/gca/pkg/logger"
	"gopkg.in/yaml.v3"
)

// ExportOptions controls an OKF bundle export run.
type ExportOptions struct {
	ProjectID string
	OutputDir string
}

// ExportReport summarizes an OKF bundle export run.
type ExportReport struct {
	Concepts int    `json:"concepts"`
	Files    int    `json:"files"`
	Duration string `json:"duration"`
}

// Export writes every OKF concept in a project's Source Store to a bundle
// directory as markdown files with YAML frontmatter, mirroring the ingest
// format in docs/designs/okf-support.md §Ingest Flow. It is the inverse of
// Ingest: the produced bundle can be re-ingested via okf.Ingest.
func Export(ctx context.Context, sa StoreAccessor, opts ExportOptions) (*ExportReport, error) {
	start := time.Now()

	source, err := sa.GetSourceStore(opts.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("okf export: source store: %w", err)
	}

	// Discover concept IDs from okf_concept facts, then bucket supporting facts.
	concepts := make(map[string]*Concept)
	for fact := range source.ScanContext(ctx, "", PredOKFConcept.String(), "") {
		if fact.Subject == "" {
			continue
		}
		concepts[fact.Subject] = &Concept{ID: fact.Subject}
	}

	if len(concepts) == 0 {
		logger.Warn("okf export: no OKF concepts found", "project", opts.ProjectID)
	} else {
		logger.Info("okf export: found concepts", "project", opts.ProjectID, "count", len(concepts))
	}

	title := make(map[string]string)
	desc := make(map[string]string)
	typ := make(map[string]string)
	resource := make(map[string]string)
	timestamp := make(map[string]string)
	body := make(map[string]string)
	frontJSON := make(map[string]string)
	tags := make(map[string][]string)
	links := make(map[string][]string)

	for fact := range source.ScanContext(ctx, "", "", "") {
		if fact.Subject == "" {
			continue
		}
		obj, _ := fact.Object.(string)
		if _, ok := concepts[fact.Subject]; !ok {
			continue
		}
		switch fact.Predicate {
		case PredOKFTitle.String():
			title[fact.Subject] = obj
		case PredOKFDescription.String():
			desc[fact.Subject] = obj
		case PredOKFConcept.String():
			typ[fact.Subject] = obj
		case PredOKFResource.String():
			resource[fact.Subject] = obj
		case PredOKFTimestamp.String():
			timestamp[fact.Subject] = obj
		case PredOKFBody.String():
			body[fact.Subject] = obj
		case PredOKFFrontmatter.String():
			frontJSON[fact.Subject] = obj
		case PredOKFTag.String():
			tags[fact.Subject] = append(tags[fact.Subject], obj)
		case PredOKFLink.String():
			links[fact.Subject] = append(links[fact.Subject], obj)
		}
	}

	// Reconstruct each concept and write it to disk.
	if opts.OutputDir == "" {
		return nil, fmt.Errorf("okf export: output_dir is required")
	}
	if err := os.MkdirAll(opts.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("okf export: create output dir: %w", err)
	}

	written := 0
	for id, c := range concepts {
		c.Title = title[id]
		c.Description = desc[id]
		c.Type = typ[id]
		c.Resource = resource[id]
		c.Timestamp = timestamp[id]
		c.Body = body[id]
		c.Tags = tags[id]
		c.Links = links[id]
		if fm, err := BodyToFrontmatter(frontJSON[id]); err == nil {
			c.Frontmatter = fm
		}

		rel := conceptSourcePath(id)
		if rel == "" {
			rel = id
		}
		rel = strings.TrimSuffix(rel, "/") + ".md"
		outPath := filepath.Join(opts.OutputDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return nil, fmt.Errorf("okf export: mkdir %s: %w", filepath.Dir(outPath), err)
		}
		raw, err := renderConceptFile(c)
		if err != nil {
			return nil, fmt.Errorf("okf export: render %s: %w", id, err)
		}
		if err := os.WriteFile(outPath, raw, 0o644); err != nil {
			return nil, fmt.Errorf("okf export: write %s: %w", outPath, err)
		}
		written++
	}

	return &ExportReport{
		Concepts: len(concepts),
		Files:    written,
		Duration: time.Since(start).String(),
	}, nil
}

// renderConceptFile serializes a concept back into OKF markdown with YAML
// frontmatter. The type field is re-derived from the config role when the
// stored okf_concept object is present; otherwise it is omitted.
func renderConceptFile(c *Concept) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("---\n")

	fm := map[string]any{}
	if c.Type != "" {
		fm["type"] = c.Type
	}
	if c.Title != "" {
		fm["title"] = c.Title
	}
	if c.Description != "" {
		fm["description"] = c.Description
	}
	if c.Resource != "" {
		fm["resource"] = c.Resource
	}
	if c.Timestamp != "" {
		fm["timestamp"] = c.Timestamp
	}
	if len(c.Tags) > 0 {
		fm["tags"] = c.Tags
	}
	for k, v := range c.Frontmatter {
		fm[k] = v
	}

	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(fm); err != nil {
		return nil, fmt.Errorf("encode frontmatter: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("close encoder: %w", err)
	}
	buf.WriteString("---\n\n")
	buf.WriteString(c.Body)
	if !bytes.HasSuffix(buf.Bytes(), []byte("\n")) {
		buf.WriteString("\n")
	}
	return buf.Bytes(), nil
}

// conceptSourcePath extracts the bundle-relative path from a concept ID of the
// form gca://project/<id>/okf/<relpath>. Returns "" when parsing fails.
func conceptSourcePath(id string) string {
	const marker = "/okf/"
	idx := strings.Index(id, marker)
	if idx < 0 {
		return ""
	}
	return id[idx+len(marker):]
}
