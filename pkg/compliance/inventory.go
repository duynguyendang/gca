package compliance

import (
	"context"
	"sort"
	"strings"

	"github.com/duynguyendang/meb"
)

// Dependency is one entry in the SBOM inventory.
type Dependency struct {
	Name    string   `json:"name"`
	Version string   `json:"version,omitempty"`
	License string   `json:"license,omitempty"`
	Files   []string `json:"files"`
}

// Inventory is the deduplicated dependency set for a project.
type Inventory struct {
	Dependencies []Dependency `json:"dependencies"`
	PackageCount int          `json:"package_count"`
}

// CollectInventory scans the source store for imports(File, Package) facts and
// returns a deduplicated, canonicalized dependency set.
func CollectInventory(ctx context.Context, source *meb.MEBStore) (*Inventory, error) {
	byPackage := map[string]*Dependency{}
	for fact := range source.ScanContext(ctx, "", "imports", "") {
		if fact.Subject == "" {
			continue
		}
		pkg, ok := fact.Object.(string)
		if !ok {
			continue
		}
		pkg = CanonicalizeImport(pkg)
		if pkg == "" {
			continue
		}
		dep, ok := byPackage[pkg]
		if !ok {
			dep = &Dependency{Name: pkg}
			byPackage[pkg] = dep
		}
		dep.Files = appendUnique(dep.Files, fact.Subject)
	}

	deps := make([]Dependency, 0, len(byPackage))
	for _, d := range byPackage {
		sort.Strings(d.Files)
		deps = append(deps, *d)
	}
	sort.Slice(deps, func(i, j int) bool { return deps[i].Name < deps[j].Name })

	return &Inventory{Dependencies: deps, PackageCount: len(deps)}, nil
}

// CanonicalizeImport normalizes an import path: strips quotes/whitespace,
// drops version suffixes for well-known schemes, and returns "" for
// non-import noise (e.g. local file references without a module path).
func CanonicalizeImport(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.Trim(s, `"'`)
	if s == "" {
		return ""
	}
	// Relative/local imports are not module dependencies.
	if strings.HasPrefix(s, ".") || strings.HasPrefix(s, "/") {
		return ""
	}
	// Go module major version suffix (github.com/x/y/v2 -> github.com/x/y).
	// Only strip when the last element is a pure semantic version.
	parts := strings.Split(s, "/")
	if len(parts) >= 2 {
		last := parts[len(parts)-1]
		if len(last) >= 2 && last[0] == 'v' {
			allDigits := true
			for _, c := range last[1:] {
				if c < '0' || c > '9' {
					allDigits = false
					break
				}
			}
			if allDigits {
				parts = parts[:len(parts)-1]
				s = strings.Join(parts, "/")
			}
		}
	}
	return s
}

func appendUnique(slice []string, v string) []string {
	for _, existing := range slice {
		if existing == v {
			return slice
		}
	}
	return append(slice, v)
}
