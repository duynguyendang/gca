package okf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/logger"
	"github.com/duynguyendang/meb"
)

// StoreAccessor provides access to the Source and Analytical stores.
type StoreAccessor interface {
	GetSourceStore(projectID string) (*meb.MEBStore, error)
	GetAnalyticalStore(projectID string) (*meb.MEBStore, error)
}

// ExportScope controls how the code graph is broken into OKF concepts.
type ExportScope string

const (
	ExportFile    ExportScope = "file"    // one concept per file (has_kind = "file")
	ExportPackage ExportScope = "package" // one concept per package (group by in_package)
	ExportCluster ExportScope = "cluster" // one concept per belongs_to_cluster
)

// markdownLinkRegex matches markdown inline links: [text](target)
var markdownLinkRegex = regexp.MustCompile(`\[[^\]\n]*\]\(([^()\s]+)(?:\s+"[^"]*")?\)`)

// convertMarkdownLinksToCode converts markdown links to code references
// Example: [Action](core/src/action.ts#Action) → `Action` or `` `core/src/action.ts#Action` ``
func convertMarkdownLinksToCode(text string) string {
	return markdownLinkRegex.ReplaceAllStringFunc(text, func(match string) string {
		// Extract the link target (between parentheses)
		parts := strings.Split(match, "](")
		if len(parts) < 2 {
			return match
		}
		target := strings.TrimSuffix(parts[1], ")")
		
		// If the target is a relative path with a symbol reference, use it as code
		if strings.Contains(target, "#") || strings.ContainsAny(target, "./") {
			return fmt.Sprintf("`%s`", target)
		}
		
		// Otherwise, extract just the last segment (e.g., "Action" from "core/src/action.ts#Action")
		if idx := strings.LastIndex(target, "/"); idx != -1 {
			return fmt.Sprintf("`%s`", target[idx+1:])
		}
		return fmt.Sprintf("`%s`", target)
	})
}

// ExportOptions controls OKF bundle export.
type ExportOptions struct {
	ProjectID        string
	OutDir           string
	Scope            ExportScope // default: ExportFile
	MaxBodyKB        int         // default 8
	IncludeSmells    bool
	IncludeCitations bool
}

// ExportReport summarizes the outcome of an export.
type ExportReport struct {
	ConceptsWritten int
	FilesWritten    int
	Duration        time.Duration
}

// Export writes the code graph as an OKF bundle. The bundle is written to a
// temp directory and atomically renamed to OutDir on success. An advisory
// lock is not implemented in v1 (no concurrent writes assumed).
func Export(ctx context.Context, sa StoreAccessor, opts ExportOptions) (*ExportReport, error) {
	start := time.Now()
	report := &ExportReport{}
	if opts.Scope == "" {
		opts.Scope = ExportFile
	}
	if opts.MaxBodyKB == 0 {
		opts.MaxBodyKB = 8
	}

	// IMPORTANT: GetAnalyticalStore (and GetSourceStore) share the same
	// underlying store instance. GetAnalyticalStore changes the topic ID on
	// it, so we must re-acquire the source store afterwards to reset it.
	sourceStore, err := sa.GetSourceStore(opts.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("okf export: get source store: %w", err)
	}
	analyticalStore, err := sa.GetAnalyticalStore(opts.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("okf export: get analytical store: %w", err)
	}
	// Re-acquire source store to reset topic ID on the shared instance.
	sourceStore, err = sa.GetSourceStore(opts.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("okf export: get source store: %w", err)
	}

	// Collect scope units
	var units []scopeUnit

	switch opts.Scope {
	case ExportFile:
		fileSet := make(map[string]bool)
		for fact := range sourceStore.ScanContext(ctx, "", config.PredicateHasKind, "file") {
			f := fact.Subject
			if fileSet[f] {
				continue
			}
			fileSet[f] = true
			u := scopeUnit{id: f, label: f, scope: opts.Scope, files: []string{f}}
			// Collect symbols defined in this file
			for df := range sourceStore.ScanContext(ctx, f, config.PredicateDefines, "") {
				if sym, ok := df.Object.(string); ok {
					u.syms = append(u.syms, sym)
				}
			}
			units = append(units, u)
		}

	case ExportPackage:
		pkgFiles := make(map[string][]string)
		pkgSyms := make(map[string][]string)
		for fact := range sourceStore.ScanContext(ctx, "", config.PredicateInPackage, "") {
			pkg, _ := fact.Object.(string)
			sym := fact.Subject
			if pkg == "" {
				continue
			}
			pkgSyms[pkg] = append(pkgSyms[pkg], sym)
			// Find file for this symbol
			for df := range sourceStore.ScanContext(ctx, "", config.PredicateDefines, sym) {
				f := df.Subject
				pkgFiles[pkg] = append(pkgFiles[pkg], f)
				break
			}
		}
		for pkg, files := range pkgFiles {
			seen := make(map[string]bool)
			var unique []string
			for _, f := range files {
				if !seen[f] {
					seen[f] = true
					unique = append(unique, f)
				}
			}
			sort.Strings(unique)
			sort.Strings(pkgSyms[pkg])
			units = append(units, scopeUnit{
				id:    pkg,
				label: pkg,
				scope: opts.Scope,
				files: unique,
				syms:  pkgSyms[pkg],
			})
		}

	case ExportCluster:
		clusterNodes := make(map[string][]string)
		for fact := range analyticalStore.ScanContext(ctx, "", "belongs_to_cluster", "") {
			cluster, _ := fact.Object.(string)
			if cluster == "" {
				continue
			}
			clusterNodes[cluster] = append(clusterNodes[cluster], fact.Subject)
		}
		for cluster, nodes := range clusterNodes {
			sort.Strings(nodes)
			var syms []string
			for _, n := range nodes {
				// Check if this node has has_kind != "file"
				for kf := range sourceStore.ScanContext(ctx, n, config.PredicateHasKind, "") {
					if k, ok := kf.Object.(string); ok && k != "file" {
						syms = append(syms, n)
						break
					}
				}
			}
			units = append(units, scopeUnit{
				id:    cluster,
				label: cluster,
				scope: opts.Scope,
				files: nodes,
				syms:  syms,
			})
		}
	}

	sort.Slice(units, func(i, j int) bool { return units[i].id < units[j].id })

	// Write to temp dir, then rename
	tmpDir := opts.OutDir + ".tmp"
	os.RemoveAll(tmpDir)
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return nil, fmt.Errorf("okf export: mkdir temp: %w", err)
	}
	defer os.RemoveAll(tmpDir) // cleanup on failure

	for _, u := range units {
		if err := exportScopeUnit(ctx, sourceStore, analyticalStore, opts, tmpDir, u.id, u.label, u.files, u.syms); err != nil {
			logger.Warn("okf export: failed to write concept", "id", u.id, "error", err)
			continue
		}
		report.ConceptsWritten++
	}

	// Write root index.md
	if err := writeIndexMD(tmpDir, units); err != nil {
		logger.Warn("okf export: failed to write root index.md", "error", err)
	}
	report.FilesWritten = 1 + report.ConceptsWritten // index.md + concepts

	// Write root log.md
	logContent := fmt.Sprintf("# Directory Update Log\n\n## %s\n\n* **Export**: Exported %d concepts from GCA project `%s`.\n",
		time.Now().Format("2006-01-02"), report.ConceptsWritten, opts.ProjectID)
	os.WriteFile(filepath.Join(tmpDir, "log.md"), []byte(logContent), 0o644) //nolint:errcheck
	report.FilesWritten++

	// Atomic rename
	os.RemoveAll(opts.OutDir)
	if err := os.Rename(tmpDir, opts.OutDir); err != nil {
		return nil, fmt.Errorf("okf export: rename: %w", err)
	}

	report.Duration = time.Since(start)
	logger.Info("OKF export complete",
		"project", opts.ProjectID,
		"scope", opts.Scope,
		"concepts", report.ConceptsWritten,
		"files", report.FilesWritten,
		"duration", report.Duration,
	)
	return report, nil
}

func exportScopeUnit(ctx context.Context, src, anal *meb.MEBStore, opts ExportOptions,
	tmpDir, unitID, label string, files, syms []string) error {

	// Synthesize frontmatter
	fm := map[string]any{
		"type": scopeType(opts.Scope),
	}
	fm["title"] = label

	// Description: first has_doc of any symbol, or placeholder
	var desc string
	for _, sym := range syms {
		for df := range src.ScanContext(ctx, sym, config.PredicateHasDoc, "") {
			if d, ok := df.Object.(string); ok && d != "" {
				desc = d
				break
			}
		}
		if desc != "" {
			break
		}
	}
	if desc == "" {
		desc = fmt.Sprintf("GCA %s: %s", opts.Scope, label)
	}
	// Convert markdown links to code references
	desc = convertMarkdownLinksToCode(desc)
	fm["description"] = desc

	// Resource: stable internal URI
	switch opts.Scope {
	case ExportFile:
		topSym := ""
		if len(syms) > 0 {
			topSym = syms[0]
		}
		if topSym != "" {
			fm["resource"] = fmt.Sprintf("%s%s/file/%s#%s", ConceptIDPrefix, opts.ProjectID, unitID, symName(topSym))
		} else {
			fm["resource"] = fmt.Sprintf("%s%s/file/%s", ConceptIDPrefix, opts.ProjectID, unitID)
		}
	case ExportPackage:
		fm["resource"] = fmt.Sprintf("%s%s/package/%s", ConceptIDPrefix, opts.ProjectID, unitID)
	case ExportCluster:
		fm["resource"] = fmt.Sprintf("%s%s/cluster/%s", ConceptIDPrefix, opts.ProjectID, unitID)
	}

	// Tags: from has_tag on any symbol in this unit
	tags := make(map[string]bool)
	for _, sym := range syms {
		for tf := range src.ScanContext(ctx, sym, config.PredicateHasTag, "") {
			if t, ok := tf.Object.(string); ok {
				tags[t] = true
			}
		}
	}
	var tagList []string
	for t := range tags {
		tagList = append(tagList, t)
	}
	sort.Strings(tagList)
	if len(tagList) > 0 {
		fm["tags"] = tagList
	}

	// Timestamp: use current time as the export timestamp
	fm["timestamp"] = time.Now().UTC().Format(time.RFC3339)

	// Extension keys from Analytical Store
	inDeg := loadAnalInt(anal, ctx, unitID, "has_in_degree")
	outDeg := loadAnalInt(anal, ctx, unitID, "has_out_degree")
	centrality := loadAnalFloat(anal, ctx, unitID, "has_centrality")
	if inDeg > 0 {
		fm["gca_in_degree"] = inDeg
	}
	if outDeg > 0 {
		fm["gca_out_degree"] = outDeg
	}
	if centrality > 0 {
		fm["gca_centrality"] = centrality
	}

	// Smells
	if opts.IncludeSmells {
		smells := loadAnalStrings(anal, ctx, unitID, "has_smell")
		if len(smells) > 0 {
			fm["gca_smells"] = smells
		}
	}

	// Synthesize body
	var body strings.Builder
	body.WriteString("# Overview\n\n")
	if desc != "" {
		body.WriteString(desc)
		body.WriteString("\n\n")
	}

	// Schema section
	if len(syms) > 0 {
		body.WriteString("# Schema\n\n")
		body.WriteString("| Symbol | Kind | Role |\n")
		body.WriteString("|--------|------|------|\n")
		for _, sym := range syms {
			kind := loadFactString(src, ctx, sym, config.PredicateHasKind)
			role := loadFactString(src, ctx, sym, config.PredicateHasRole)
			if kind == "" {
				kind = "symbol"
			}
			if role == "" {
				role = "-"
			}
			body.WriteString(fmt.Sprintf("| `%s` | %s | %s |\n", symName(sym), kind, role))
		}
		body.WriteString("\n")
	}

	// Calls section
	body.WriteString("# Calls\n\n")
	callCount := 0
	for _, sym := range syms {
		for cf := range src.ScanContext(ctx, sym, config.PredicateCalls, "") {
			if callee, ok := cf.Object.(string); ok {
				body.WriteString(fmt.Sprintf("- `%s` → `%s`\n", symName(sym), symName(callee)))
				callCount++
				if callCount >= 20 {
					break
				}
			}
		}
		if callCount >= 20 {
			break
		}
	}
	if callCount == 0 {
		body.WriteString("_No outbound calls._\n")
	}
	body.WriteString("\n")

	// Called By section
	body.WriteString("# Called By\n\n")
	calledCount := 0
	for _, sym := range syms {
		for cf := range src.ScanContext(ctx, "", config.PredicateCalls, sym) {
			if cf.Subject != "" {
				callerID := cf.Subject
				body.WriteString(fmt.Sprintf("- `%s` ← `%s`\n", symName(sym), symName(callerID)))
				calledCount++
				if calledCount >= 20 {
					break
				}
			}
		}
		if calledCount >= 20 {
			break
		}
	}
	if calledCount == 0 {
		body.WriteString("_No inbound calls._\n")
	}
	body.WriteString("\n")

	// Bridges section (bridges_to facts for concepts that bridge to this unit)
	body.WriteString("# Bridges\n\n")
	bridgeCount := 0
	for bf := range anal.ScanContext(ctx, "", "bridges_to", unitID) {
		conceptID := bf.Subject
		body.WriteString(fmt.Sprintf("- Bridged from: `%s`\n", conceptID))
		bridgeCount++
	}
	// Also: bridges FROM this unit's OKF concepts (if re-ingested)
	for bf := range anal.ScanContext(ctx, unitID, "bridges_to", "") {
		symbolID, _ := bf.Object.(string)
		body.WriteString(fmt.Sprintf("- Bridges to: `%s`\n", symbolID))
		bridgeCount++
	}
	if bridgeCount == 0 {
		body.WriteString("_No bridges._\n")
	}
	body.WriteString("\n")

	// Citations section (URL-only links from OKF concepts bridged to this unit, per OKF §8)
	var citations []string
	seen := make(map[string]bool)
	for bf := range anal.ScanContext(ctx, "", "bridges_to", unitID) {
		conceptID := bf.Subject
		for lf := range src.ScanContext(ctx, conceptID, "okf_link", "") {
			if target, ok := lf.Object.(string); ok && IsExternalURL(target) && !seen[target] {
				citations = append(citations, target)
				seen[target] = true
			}
		}
	}
	if len(citations) > 0 {
		body.WriteString("# Citations\n\n")
		for i, c := range citations {
			body.WriteString(fmt.Sprintf("[%d] %s\n", i+1, c))
		}
		body.WriteString("\n")
	}

	// Build frontmatter YAML
	fmYAML := marshalFrontmatter(fm)

	// Write file
	outPath := filepath.Join(tmpDir, scopeDir(opts.Scope), safeFilename(unitID)+".md")
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	content := fmt.Sprintf("---\n%s---\n\n%s", fmYAML, body.String())
	return os.WriteFile(outPath, []byte(content), 0o644)
}

func scopeType(s ExportScope) string {
	switch s {
	case ExportFile:
		return "GCA File"
	case ExportPackage:
		return "GCA Package"
	case ExportCluster:
		return "GCA Cluster"
	}
	return "GCA Concept"
}

func scopeDir(s ExportScope) string {
	switch s {
	case ExportFile:
		return "files"
	case ExportPackage:
		return "packages"
	case ExportCluster:
		return "clusters"
	}
	return "concepts"
}

func safeFilename(id string) string {
	s := strings.ReplaceAll(id, "/", "__")
	s = strings.ReplaceAll(s, ":", "_")
	s = strings.ReplaceAll(s, `\`, "__")
	return s
}

func symName(id string) string {
	if idx := strings.LastIndex(id, ":"); idx >= 0 {
		return id[idx+1:]
	}
	return filepath.Base(id)
}

func loadFactString(store *meb.MEBStore, ctx context.Context, subject, predicate string) string {
	for f := range store.ScanContext(ctx, subject, predicate, "") {
		if s, ok := f.Object.(string); ok {
			return s
		}
	}
	return ""
}

func loadAnalInt(store *meb.MEBStore, ctx context.Context, subject, predicate string) int {
	s := loadFactString(store, ctx, subject, predicate)
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}

func loadAnalFloat(store *meb.MEBStore, ctx context.Context, subject, predicate string) float64 {
	s := loadFactString(store, ctx, subject, predicate)
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

func loadAnalStrings(store *meb.MEBStore, ctx context.Context, subject, predicate string) []string {
	var out []string
	for f := range store.ScanContext(ctx, subject, predicate, "") {
		if s, ok := f.Object.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func marshalFrontmatter(m map[string]any) string {
	var b strings.Builder
	// Write keys in a stable order
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := m[k]
		switch val := v.(type) {
		case string:
			b.WriteString(fmt.Sprintf("%s: %s\n", k, yamlQuote(val)))
		case int:
			b.WriteString(fmt.Sprintf("%s: %d\n", k, val))
		case float64:
			b.WriteString(fmt.Sprintf("%s: %.6f\n", k, val))
		case []string:
			b.WriteString(fmt.Sprintf("%s:\n", k))
			for _, s := range val {
				b.WriteString(fmt.Sprintf("  - %s\n", yamlQuote(s)))
			}
		default:
			b.WriteString(fmt.Sprintf("%s: %v\n", k, val))
		}
	}
	return b.String()
}

func yamlQuote(s string) string {
	if strings.ContainsAny(s, ":#{}[]&*?|>!%@`") || s == "" {
		return fmt.Sprintf("%q", s)
	}
	return s
}

// scopeUnit represents a single unit (file/package/cluster) being exported.
type scopeUnit struct {
	id    string // file path or package name or cluster ID
	label string // human-readable name for the file
	scope ExportScope
	files []string
	syms  []string
}

func writeIndexMD(dir string, units []scopeUnit) error {
	var b strings.Builder
	// Write frontmatter with okf_version per OKF §11
	b.WriteString("---\nokf_version: \"0.1\"\n---\n\n")
	for _, unit := range units {
		b.WriteString(fmt.Sprintf("* [%s](%s/%s.md)\n", unit.label, scopeDir(unit.scope), safeFilename(unit.id)))
	}
	return os.WriteFile(filepath.Join(dir, "index.md"), []byte(b.String()), 0o644)
}
