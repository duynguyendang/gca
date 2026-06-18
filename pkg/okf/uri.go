package okf

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// GCAURI is a parsed gca:// URI used as a stable cross-reference identifier.
//
//	Internal concept ID:    gca://project/<projectID>/okf/<bundleRelPath>
//	Export resource URI:    gca://project/<projectID>/file/<relPath>#<symbolName>
//	Code-path link (explicit): gca://project/<projectID>/file/<filePath>#<symbol>
type GCAURI struct {
	ProjectID  string
	Kind       string // "okf" | "file"
	Path       string // bundle-relative or file-relative
	SymbolName string // empty if no fragment
}

// String formats the URI back to its canonical form.
func (u GCAURI) String() string {
	base := fmt.Sprintf("%s%s/%s/%s", ConceptIDPrefix, u.ProjectID, u.Kind, u.Path)
	if u.SymbolName != "" {
		base += "#" + u.SymbolName
	}
	return base
}

// ParseGCAURI parses a gca:// URI. Returns ok=false for non-gca:// inputs.
func ParseGCAURI(raw string) (GCAURI, bool) {
	if !strings.HasPrefix(raw, "gca://") {
		return GCAURI{}, false
	}
	rest := strings.TrimPrefix(raw, "gca://")
	pathPart, frag, _ := strings.Cut(rest, "#")

	// pathPart: project/<id>/<kind>/<path...>
	segs := strings.SplitN(pathPart, "/", 4)
	if len(segs) < 4 || segs[0] != "project" {
		return GCAURI{}, false
	}
	return GCAURI{
		ProjectID:  segs[1],
		Kind:       segs[2],
		Path:       segs[3],
		SymbolName: frag,
	}, true
}

// SplitPathFragment splits "path#symbol" into (path, symbol). The path is
// forward-slashed and any leading "/" stripped.
func SplitPathFragment(raw string) (string, string, bool) {
	path, frag, ok := strings.Cut(raw, "#")
	if !ok || frag == "" {
		return "", "", false
	}
	path = strings.TrimPrefix(path, "/")
	path = filepath.ToSlash(path)
	if path == "" {
		return "", "", false
	}
	return path, frag, true
}

// BQResource is a parsed BigQuery resource URI, e.g.
//   https://bigquery.googleapis.com/v2/projects/<p>/datasets/<d>/tables/<t>
type BQResource struct {
	Project  string
	Dataset  string
	Table    string // empty for dataset-level
	Raw      string
}

// ParseBQResource parses a BigQuery console/API URI. Returns ok=false for
// non-BigQuery inputs. Supports both forms:
//   - https://bigquery.googleapis.com/v2/projects/<p>/datasets/<d>[/tables/<t>]
//   - https://console.cloud.google.com/bigquery?p=<p>&d=<d>&t=<t>
func ParseBQResource(raw string) (BQResource, bool) {
	if !strings.Contains(raw, "bigquery") {
		return BQResource{}, false
	}
	// Form 1: API URL
	if strings.HasPrefix(raw, "https://bigquery.googleapis.com/") {
		parts := strings.Split(raw, "/")
		var r BQResource
		r.Raw = raw
		for i, p := range parts {
			if p == "projects" && i+1 < len(parts) {
				r.Project = parts[i+1]
			}
			if p == "datasets" && i+1 < len(parts) {
				r.Dataset = parts[i+1]
			}
			if p == "tables" && i+1 < len(parts) {
				r.Table = parts[i+1]
			}
		}
		if r.Project != "" && r.Dataset != "" {
			return r, true
		}
	}
	// Form 2: console URL with query params
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		q := u.Query()
		if p := q.Get("p"); p != "" {
			r := BQResource{
				Project: p,
				Dataset: q.Get("d"),
				Table:   q.Get("t"),
				Raw:     raw,
			}
			if r.Dataset != "" {
				return r, true
			}
		}
	}
	return BQResource{}, false
}

// IsExternalURL reports whether the link target is an external URL (http/https).
func IsExternalURL(raw string) bool {
	return strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://")
}

// IsBundleAbsolute reports whether the link target is bundle-absolute (starts with "/").
func IsBundleAbsolute(raw string) bool {
	return strings.HasPrefix(raw, "/")
}

// IsRelative reports whether the link target is relative (starts with "./" or "../").
func IsRelative(raw string) bool {
	return strings.HasPrefix(raw, "./") || strings.HasPrefix(raw, "../")
}