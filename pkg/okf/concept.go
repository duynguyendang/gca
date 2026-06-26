// Package okf implements OKF (Open Knowledge Format v0.1) ingestion and export
// for GCA. See docs/designs/okf-support.md for the full design.
package okf

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
)

// ConceptIDPrefix is the namespace prefix for OKF concept IDs in the Source Store.
// A concept from project "acme" at bundle path "tables/orders" gets the ID:
//   gca://project/acme/okf/tables/orders
const ConceptIDPrefix = "gca://project/"

// Concept represents one parsed OKF concept document. The parser produces it
// from a markdown file; the ingestor converts it to Source/Analytical Store facts.
type Concept struct {
	ID            string            // gca://project/<id>/okf/<bundleRelPath>
	Type          string            // required by OKF §9.1
	Title         string
	Description   string
	Resource      string            // optional canonical URI for the underlying asset
	Tags          []string
	Timestamp     string            // ISO 8601, optional
	Body          string            // full markdown body (without frontmatter)
	Links         []string          // raw markdown link targets from body (excluding citations)
	Citations     []string          // raw markdown link targets from # Citations section
	SourcePath    string            // bundle-relative path, e.g. "tables/orders.md"
	ContentHash   string            // sha256 of the raw file content
	Frontmatter   map[string]any    // preserved extension keys (not in the well-known set)
}

// ConceptID builds the internal concept ID from a project ID and a bundle-relative
// path. BundleRelPath is normalized: forward slashes, leading "./" stripped,
// trailing ".md" stripped, lowercase.
//
//   ConceptID("acme", "tables/orders.md") → "gca://project/acme/okf/tables/orders"
//   ConceptID("acme", "./foo/bar.md")    → "gca://project/acme/okf/foo/bar"
func ConceptID(projectID, bundleRelPath string) string {
	p := strings.TrimPrefix(bundleRelPath, "./")
	p = strings.TrimSuffix(p, ".md")
	p = strings.ReplaceAll(p, `\`, "/")
	p = filepath.ToSlash(p)
	return fmt.Sprintf("%s%s/okf/%s", ConceptIDPrefix, projectID, p)
}

// IsReservedFile reports whether a bundle-relative filename is reserved by
// OKF §3.1 and must NOT be treated as a concept.
func IsReservedFile(name string) bool {
	base := filepath.Base(name)
	return base == "index.md" || base == "log.md"
}

// HashContent returns a sha256 hex digest of the given content.
func HashContent(content []byte) string {
	h := sha256.Sum256(content)
	return hex.EncodeToString(h[:])
}

// IsWellKnownExtensionKey reports whether the frontmatter key is one of the
// GCA-specific keys that we recognize and map to analytical predicates on
// re-ingest. Anything else is preserved into okf_frontmatter as JSON.
func IsWellKnownExtensionKey(key string) bool {
	switch key {
	case "gca_in_degree", "gca_centrality", "gca_smells":
		return true
	}
	return false
}