package okf

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/duynguyendang/meb"
)

// ResolvedLink is the outcome of resolving one markdown link target against
// the Source Store and the bundle's concept map.
type ResolvedLink struct {
	Raw       string // original link target, e.g. "/tables/orders.md" or "src/foo.go#Bar"
	Target    string // canonical ID or URI stored as the Object of okf_link(C, T)
	SymbolID  string // non-empty only for code-path links that resolved to a Source Store symbol
	IsBridge  bool   // true when SymbolID is set and bridges_to(C, SymbolID) should be emitted
}

// LinkResolver resolves raw markdown link targets to canonical IDs and emits
// bridges_to facts when a code-path link matches a Source Store symbol.
type LinkResolver struct {
	ProjectID  string
	Store      *meb.MEBStore
	ConceptIDs map[string]string // bundleRelPath (e.g. "tables/orders") → concept ID
}

// NewLinkResolver constructs a resolver with the project's concept map pre-populated.
// Concepts should be all ConceptIDs in the bundle (built during ingest step 1).
func NewLinkResolver(projectID string, store *meb.MEBStore, concepts []*Concept) *LinkResolver {
	m := make(map[string]string, len(concepts))
	for _, c := range concepts {
		// Map bundle-relative path → concept ID. Strip leading "./" and trailing ".md".
		key := strings.TrimPrefix(c.SourcePath, "./")
		key = strings.TrimSuffix(key, ".md")
		key = filepath.ToSlash(key)
		m[key] = c.ID
	}
	return &LinkResolver{
		ProjectID:  projectID,
		Store:      store,
		ConceptIDs: m,
	}
}

// Resolve resolves a single raw markdown link target. The fromConceptDir is
// the bundle-relative directory of the linking concept (used for relative
// link resolution); pass "" if the linking concept is at the bundle root.
func (r *LinkResolver) Resolve(ctx context.Context, raw, fromConceptDir string) ResolvedLink {
	raw = strings.TrimSpace(raw)

	// 1. Bundle-absolute: "/tables/orders.md"
	if IsBundleAbsolute(raw) {
		target := strings.TrimPrefix(raw, "/")
		target = strings.TrimSuffix(target, ".md")
		if conceptID, ok := r.ConceptIDs[target]; ok {
			return ResolvedLink{Raw: raw, Target: conceptID}
		}
		// Bundle-absolute path that doesn't resolve to a concept: store as-is.
		return ResolvedLink{Raw: raw, Target: raw}
	}

	// 2. Relative: "./foo.md" or "../sibling/bar.md"
	if IsRelative(raw) {
		target := strings.TrimSuffix(raw, ".md")
		resolved := resolveRelative(fromConceptDir, target)
		resolved = filepath.ToSlash(resolved)
		if conceptID, ok := r.ConceptIDs[resolved]; ok {
			return ResolvedLink{Raw: raw, Target: conceptID}
		}
		// Relative link to a non-concept (e.g. an image in assets/): store the resolved path.
		return ResolvedLink{Raw: raw, Target: "/" + resolved + ".md"}
	}

	// 3. External URL: "http(s)://..."
	if IsExternalURL(raw) {
		// External URLs are stored as the URL string itself. The ingestor's
		// resource-matching pass may rewrite to a concept ID if some concept
		// declared the same URL as its `resource`.
		return ResolvedLink{Raw: raw, Target: raw}
	}

	// 4. Code-path link (with or without gca:// scheme).
	return r.resolveCodeLink(ctx, raw)
}

// resolveCodeLink tries to find a Source Store symbol matching a code-path link.
// Returns (symbolID, okfURI, found).
func (r *LinkResolver) resolveCodeLink(ctx context.Context, raw string) ResolvedLink {
	// Form A: gca:// scheme (explicit)
	if parsed, ok := ParseGCAURI(raw); ok && parsed.Kind == "file" {
		okfURI := parsed.String()
		symID := r.lookupSymbol(ctx, parsed.Path, parsed.SymbolName)
		return ResolvedLink{
			Raw:      raw,
			Target:   okfURI,
			SymbolID: symID,
			IsBridge: symID != "",
		}
	}

	// Form B: path#symbol (relative or absolute)
	if filePath, symName, ok := SplitPathFragment(raw); ok {
		okfURI := fmt.Sprintf("%s%s/file/%s#%s", ConceptIDPrefix, r.ProjectID, filePath, symName)
		symID := r.lookupSymbol(ctx, filePath, symName)
		return ResolvedLink{
			Raw:      raw,
			Target:   okfURI,
			SymbolID: symID,
			IsBridge: symID != "",
		}
	}

	// Not a code-path link we recognize. Store as a literal string so we
	// don't lose it — the parser only emits links it found via regex, so
	// this branch is rare (e.g. an unusual protocol like mailto:).
	return ResolvedLink{Raw: raw, Target: raw}
}

// lookupSymbol finds a Source Store symbol by file path and name. It uses
// defines(File, Symbol) joined with has_name(Symbol, Name) — no string-prefix
// matching on the symbol name itself.
func (r *LinkResolver) lookupSymbol(ctx context.Context, filePath, symName string) string {
	if r.Store == nil || symName == "" {
		return ""
	}
	for fact, err := range r.Store.ScanContext(ctx, filePath, "defines", "") {
		if err != nil {
			continue
		}
		symID, ok := fact.Object.(string)
		if !ok || symID == "" {
			continue
		}
		for nf := range r.Store.ScanContext(ctx, symID, "has_name", "") {
			if name, ok := nf.Object.(string); ok && name == symName {
				return symID
			}
		}
	}
	return ""
}

// resolveRelative resolves a relative path against a directory. Both inputs
// use forward slashes. The result is forward-slashed.
func resolveRelative(fromDir, rel string) string {
	if fromDir == "" {
		return rel
	}
	// filepath.Join handles ".." correctly on the OS but normalizes separators;
	// we convert back to forward slashes for storage.
	return filepath.ToSlash(filepath.Join(fromDir, rel))
}

// DirOfConcept returns the bundle-relative directory containing the given
// bundle-relative path. Empty string if the path is at the bundle root.
func DirOfConcept(bundleRelPath string) string {
	dir := filepath.Dir(bundleRelPath)
	if dir == "." {
		return ""
	}
	return filepath.ToSlash(dir)
}