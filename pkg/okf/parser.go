package okf

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// frontmatterDelim is the YAML frontmatter delimiter per OKF §4.1.
var frontmatterDelim = []byte("---")

// ErrMissingType is returned by ParseConcept when the frontmatter is missing
// the required `type` field (OKF §9.1 — non-conformant bundle).
var ErrMissingType = errors.New("okf: frontmatter missing required 'type' field")

// ErrInvalidFrontmatter is returned when YAML parsing fails or the frontmatter
// block is malformed.
var ErrInvalidFrontmatter = errors.New("okf: invalid YAML frontmatter")

// markdownLinkRe matches markdown inline links: [text](target). It captures
// the link target in group 1. We use a non-greedy match for the text portion
// and disallow nested brackets/parentheses inside the target.
var markdownLinkRe = regexp.MustCompile(`\[[^\]\n]*\]\(([^()\s]+)(?:\s+"[^"]*")?\)`)

// ParseConcept splits raw markdown into frontmatter + body, parses the
// frontmatter as YAML, validates the OKF-required `type` field, extracts
// markdown link targets from the body, and returns a Concept ready for
// ingestion. The SourcePath is the bundle-relative path supplied by the caller
// (the parser does not walk the filesystem).
func ParseConcept(sourcePath string, raw []byte) (*Concept, error) {
	frontmatter, body, err := splitFrontmatter(raw)
	if err != nil {
		return nil, err
	}

	// Parse frontmatter as a generic YAML map so we can preserve unknown keys.
	rawMap := map[string]any{}
	if len(bytes.TrimSpace(frontmatter)) > 0 {
		if err := yaml.Unmarshal(frontmatter, &rawMap); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidFrontmatter, err)
		}
	}

	c := &Concept{
		SourcePath:  sourcePath,
		Body:        string(body),
		Frontmatter: map[string]any{},
		Tags:        []string{},
		Links:       extractLinks(body),
		ContentHash: HashContent(raw),
	}

	// Required field.
	if t, ok := rawMap["type"].(string); ok && t != "" {
		c.Type = t
	} else {
		return nil, fmt.Errorf("%w (file: %s)", ErrMissingType, sourcePath)
	}

	if v, ok := rawMap["title"].(string); ok {
		c.Title = v
	}
	if v, ok := rawMap["description"].(string); ok {
		c.Description = v
	}
	if v, ok := rawMap["resource"].(string); ok {
		c.Resource = v
	}
	if v, ok := rawMap["timestamp"].(string); ok {
		c.Timestamp = v
	}
	if v, ok := rawMap["tags"].([]any); ok {
		for _, item := range v {
			if s, ok := item.(string); ok {
				c.Tags = append(c.Tags, s)
			}
		}
	}

	// Preserve extension keys.
	for k, v := range rawMap {
		if isWellKnownFrontmatterKey(k) {
			continue
		}
		c.Frontmatter[k] = v
	}

	return c, nil
}

// ParseRootIndexFrontmatter parses only the frontmatter of a bundle-root
// index.md to extract okf_version (OKF §11). Returns "" when not present.
func ParseRootIndexFrontmatter(raw []byte) (version string, err error) {
	frontmatter, _, err := splitFrontmatter(raw)
	if err != nil {
		return "", err
	}
	if len(bytes.TrimSpace(frontmatter)) == 0 {
		return "", nil
	}
	m := map[string]any{}
	if err := yaml.Unmarshal(frontmatter, &m); err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidFrontmatter, err)
	}
	if v, ok := m["okf_version"].(string); ok {
		return v, nil
	}
	return "", nil
}

// splitFrontmatter returns (frontmatter, body). If there is no frontmatter
// block, frontmatter is empty and body is the entire input.
func splitFrontmatter(raw []byte) ([]byte, []byte, error) {
	// OKF frontmatter must start with `---` on the first line.
	if !bytes.HasPrefix(raw, frontmatterDelim) {
		return nil, raw, nil
	}
	// Find the closing `---` line after the first one.
	rest := raw[len(frontmatterDelim):]
	// Skip optional leading newline.
	rest = bytes.TrimLeft(rest, "\r\n")
	idx := bytes.Index(rest, append([]byte("\n"), frontmatterDelim...))
	if idx < 0 {
		// No closing delimiter — treat as no frontmatter (still parseable as body).
		return nil, raw, nil
	}
	frontmatter := rest[:idx]
	// Skip past the closing "---\n".
	body := rest[idx+1+len(frontmatterDelim):]
	body = bytes.TrimLeft(body, "\r\n")
	return frontmatter, body, nil
}

// extractLinks returns the targets of all inline markdown links in body,
// in document order. Image links (`![alt](target)`) are also captured —
// we treat them uniformly as link targets; consumers can filter if needed.
func extractLinks(body []byte) []string {
	matches := markdownLinkRe.FindAllSubmatch(body, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) >= 2 {
			out = append(out, string(m[1]))
		}
	}
	return out
}

// isWellKnownFrontmatterKey reports whether a key is one of the OKF-spec
// keys (type, title, description, resource, tags, timestamp) or the
// GCA-specific extension keys we recognize.
func isWellKnownFrontmatterKey(k string) bool {
	switch k {
	case "type", "title", "description", "resource", "tags", "timestamp",
		"okf_version", "gca_in_degree", "gca_centrality", "gca_smells":
		return true
	}
	return false
}

// SerializeFrontmatter returns the JSON-serialized form of c.Frontmatter,
// suitable for storing as the Object of an okf_frontmatter(Concept, JSON) fact.
// Returns an empty string when Frontmatter is empty.
func (c *Concept) SerializeFrontmatter() (string, error) {
	if len(c.Frontmatter) == 0 {
		return "", nil
	}
	b, err := json.Marshal(c.Frontmatter)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// BodyToFrontmatter reconstructs the frontmatter map from a previously
// stored okf_frontmatter JSON blob. Used during re-ingest.
func BodyToFrontmatter(jsonStr string) (map[string]any, error) {
	if strings.TrimSpace(jsonStr) == "" {
		return map[string]any{}, nil
	}
	m := map[string]any{}
	if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
		return nil, fmt.Errorf("okf: invalid frontmatter JSON: %w", err)
	}
	return m, nil
}