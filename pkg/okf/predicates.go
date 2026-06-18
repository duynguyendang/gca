// Package okf implements OKF (Open Knowledge Format v0.1) ingestion and export
// for GCA. See docs/designs/okf-support.md for the full design.
package okf

// Predicate is a typed wrapper around an OKF predicate name. All OKF predicates
// are kept in sync with policies/okf/_decl.mg; the registry validates declarations
// against known predicates at load time.
type Predicate string

const (
	// Source Store predicates (see policies/okf/_decl.mg)
	PredOKFConcept     Predicate = "okf_concept"
	PredOKFTitle       Predicate = "okf_title"
	PredOKFDescription Predicate = "okf_description"
	PredOKFResource    Predicate = "okf_resource"
	PredOKFTag         Predicate = "okf_tag"
	PredOKFTimestamp   Predicate = "okf_timestamp"
	PredOKFBody        Predicate = "okf_body"
	PredOKFLink        Predicate = "okf_link"
	PredOKFContentHash Predicate = "okf_content_hash"
	PredOKFFrontmatter Predicate = "okf_frontmatter"
	PredOKFVersion     Predicate = "okf_version"

	// Analytical Store predicates
	PredBridgesTo     Predicate = "bridges_to"
	PredOKFBridgeMiss Predicate = "okf_bridge_miss"
)

// String returns the predicate name as a plain string.
func (p Predicate) String() string {
	return string(p)
}