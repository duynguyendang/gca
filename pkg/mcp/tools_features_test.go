package mcp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/compliance"
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/ingest"
	mebpkg "github.com/duynguyendang/meb"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

// newFeatureSeededManager seeds a testproj with the facts the F2-F5 tools need:
// KPI snapshots (trends), vulnerability facts (compliance), and imports (SBOM).
func newFeatureSeededManager(t *testing.T) *manager.StoreManager {
	t.Helper()
	mgr := newSeededManager(t, false)

	an, err := mgr.GetAnalyticalStore("testproj")
	require.NoError(t, err)

	now := time.Now().UTC().Truncate(time.Millisecond)
	snap1, _ := json.Marshal(ingest.KPISnapshot{
		ID:          "kpi:testproj:aaa",
		CommitSHA:   "aaa",
		Timestamp:   now.Add(-2 * time.Hour),
		HealthScore: 80,
		HealthDebt:  500,
		SmellCount:  20,
	})
	snap2, _ := json.Marshal(ingest.KPISnapshot{
		ID:          "kpi:testproj:bbb",
		CommitSHA:   "bbb",
		Timestamp:   now.Add(-time.Hour),
		HealthScore: 90,
		HealthDebt:  300,
		SmellCount:  12,
	})
	snap3, _ := json.Marshal(ingest.KPISnapshot{
		ID:          "kpi:testproj:ccc",
		CommitSHA:   "ccc",
		Timestamp:   now,
		HealthScore: 95,
		HealthDebt:  150,
		SmellCount:  5,
	})
	require.NoError(t, an.AddFactBatch([]mebpkg.Fact{
		{Subject: "kpi:testproj:aaa", Predicate: config.PredicateKPISnapshot, Object: string(snap1)},
		{Subject: "kpi:testproj:bbb", Predicate: config.PredicateKPISnapshot, Object: string(snap2)},
		{Subject: "kpi:testproj:ccc", Predicate: config.PredicateKPISnapshot, Object: string(snap3)},
		{Subject: "golang.org/x/net", Predicate: compliance.PredicateHasVulnerability, Object: "GHSA-vvpx-go8f-jh44"},
		{Subject: "golang.org/x/net", Predicate: compliance.PredicateHasVulnerability, Object: "GHSA-4374-p667-p6c8"},
		{Subject: "GHSA-vvpx-go8f-jh44", Predicate: compliance.PredicateVulnSeverity, Object: "high"},
		{Subject: "GHSA-vvpx-go8f-jh44", Predicate: compliance.PredicateVulnSummary, Object: "HTTP/2 rapid reset can cause excessive resource consumption"},
		{Subject: "GHSA-4374-p667-p6c8", Predicate: compliance.PredicateVulnSeverity, Object: "medium"},
		{Subject: "GHSA-4374-p667-p6c8", Predicate: compliance.PredicateVulnSummary, Object: "HTTP/2 stream cancellation"},
	}))

	src, err := mgr.GetSourceStore("testproj")
	require.NoError(t, err)
	require.NoError(t, src.AddFactBatch([]mebpkg.Fact{
		{Subject: "file:a.go", Predicate: config.PredicateImports, Object: "\"golang.org/x/net/http2\""},
		{Subject: "file:a.go", Predicate: config.PredicateImports, Object: "\"fmt\""},
		{Subject: "file:b.go", Predicate: config.PredicateImports, Object: "\"golang.org/x/net/http2\""},
	}))
	return mgr
}

// newFeatureClient builds an in-process client over a feature-seeded manager.
func newFeatureClient(t *testing.T) (*mcpclient.Client, context.CancelFunc) {
	t.Helper()
	mgr := newFeatureSeededManager(t)
	ms := New(Options{Manager: mgr})
	client, err := mcpclient.NewInProcessClient(ms)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	_, err = client.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "gca-feature-test", Version: "1.0.0"},
			Capabilities:    mcp.ClientCapabilities{},
		},
	})
	require.NoError(t, err)
	return client, func() {
		cancel()
		client.Close()
	}
}

func callFeature(t *testing.T, client *mcpclient.Client, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := client.CallTool(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: name, Arguments: args},
	})
	require.NoError(t, err)
	return res
}

func TestMCPServer_ListToolsIncludesFeatureWave(t *testing.T) {
	client, closeFn := newFeatureClient(t)
	defer closeFn()
	res, err := client.ListTools(context.Background(), mcp.ListToolsRequest{})
	require.NoError(t, err)
	names := map[string]bool{}
	for _, tool := range res.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{"get_trends", "get_impact_report", "list_vulnerabilities", "get_sbom", "get_architecture_report"} {
		require.True(t, names[want], "missing tool %s", want)
	}
}

func TestMCPGetTrends(t *testing.T) {
	client, closeFn := newFeatureClient(t)
	defer closeFn()

	res := callFeature(t, client, "get_trends", map[string]any{"project": "testproj"})
	require.False(t, res.IsError)
	var body struct {
		ProjectID string `json:"project_id"`
		Metric    string `json:"metric"`
		Points    []struct {
			Value int `json:"value"`
		} `json:"points"`
		Summary struct {
			Start int    `json:"start"`
			End   int    `json:"end"`
			Delta int    `json:"delta"`
			Trend string `json:"trend"`
		} `json:"summary"`
	}
	require.NoError(t, json.Unmarshal([]byte(resultText(t, res)), &body))
	require.Equal(t, "testproj", body.ProjectID)
	require.Equal(t, "health", body.Metric)
	require.Len(t, body.Points, 3)
	require.Equal(t, 80, body.Summary.Start)
	require.Equal(t, 95, body.Summary.End)
	require.Equal(t, 15, body.Summary.Delta)
	require.Equal(t, "improving", body.Summary.Trend)

	// Unknown project => project_not_found envelope.
	errRes := callFeature(t, client, "get_trends", map[string]any{"project": "nope"})
	env := decodeErr(t, errRes)
	require.Equal(t, ErrCodeProjectNotFound, env.Code)
}

func TestMCPGetImpactReport(t *testing.T) {
	client, closeFn := newFeatureClient(t)
	defer closeFn()

	diff := "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1,1 +1,1 @@\n-funcA()\n+funcA2()\n"
	res := callFeature(t, client, "get_impact_report", map[string]any{
		"project": "testproj",
		"diff":    diff,
	})
	require.False(t, res.IsError)
	var body struct {
		TouchedFiles      []string       `json:"touched_files"`
		SmellsNew         map[string]int `json:"smells_new"`
		ReachableCallers  int            `json:"reachable_callers_count"`
		EntryPointsAffect []string       `json:"entry_points_affected"`
	}
	require.NoError(t, json.Unmarshal([]byte(resultText(t, res)), &body))
	require.GreaterOrEqual(t, len(body.SmellsNew), 0)

	// Missing diff is invalid_argument.
	errRes := callFeature(t, client, "get_impact_report", map[string]any{"project": "testproj"})
	env := decodeErr(t, errRes)
	require.Equal(t, ErrCodeInvalidArgument, env.Code)
}

func TestMCPListVulnerabilities(t *testing.T) {
	client, closeFn := newFeatureClient(t)
	defer closeFn()

	res := callFeature(t, client, "list_vulnerabilities", map[string]any{"project": "testproj"})
	require.False(t, res.IsError)
	var body struct {
		Vulnerabilities []struct {
			Package    string `json:"package"`
			AdvisoryID string `json:"advisory_id"`
			Severity   string `json:"severity"`
		} `json:"vulnerabilities"`
		Total int `json:"total"`
	}
	require.NoError(t, json.Unmarshal([]byte(resultText(t, res)), &body))
	require.Equal(t, 2, body.Total)
	require.Equal(t, "golang.org/x/net", body.Vulnerabilities[0].Package)

	// Severity filter narrows to high only.
	res2 := callFeature(t, client, "list_vulnerabilities", map[string]any{
		"project":  "testproj",
		"severity": "high",
	})
	require.False(t, res2.IsError)
	var body2 struct {
		Vulnerabilities []struct {
			Severity string `json:"severity"`
		} `json:"vulnerabilities"`
		Total int `json:"total"`
	}
	require.NoError(t, json.Unmarshal([]byte(resultText(t, res2)), &body2))
	require.Equal(t, 1, body2.Total)
	require.Equal(t, "high", body2.Vulnerabilities[0].Severity)
}

func TestMCPGetSBOM(t *testing.T) {
	client, closeFn := newFeatureClient(t)
	defer closeFn()

	res := callFeature(t, client, "get_sbom", map[string]any{"project": "testproj"})
	require.False(t, res.IsError)
	var body struct {
		PackageCount int `json:"package_count"`
		Dependencies []struct {
			Name string `json:"name"`
		} `json:"dependencies"`
	}
	require.NoError(t, json.Unmarshal([]byte(resultText(t, res)), &body))
	require.Equal(t, 2, body.PackageCount)

	// CycloneDX format.
	res2 := callFeature(t, client, "get_sbom", map[string]any{"project": "testproj", "format": "cyclonedx"})
	require.False(t, res2.IsError)
	var body2 struct {
		BOMFormat  string `json:"bomFormat"`
		Components []struct {
			Name string `json:"name"`
		} `json:"components"`
	}
	require.NoError(t, json.Unmarshal([]byte(resultText(t, res2)), &body2))
	require.Equal(t, "CycloneDX", body2.BOMFormat)
	require.Len(t, body2.Components, 2)
}

func TestMCPGetArchitectureReport(t *testing.T) {
	client, closeFn := newFeatureClient(t)
	defer closeFn()

	res := callFeature(t, client, "get_architecture_report", map[string]any{"project": "testproj"})
	require.False(t, res.IsError)
	require.Contains(t, resultText(t, res), "Architecture Report")
}
