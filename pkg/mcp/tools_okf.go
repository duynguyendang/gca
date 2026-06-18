package mcp

import (
	"context"
	"fmt"

	"github.com/duynguyendang/gca/pkg/okf"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// AddOKFTools registers OKF ingest and export tools on the MCP server.
// The storeManager provides access to both Source and Analytical stores.
func AddOKFTools(s *server.MCPServer, storeMgr okf.StoreAccessor, dataDir string) {
	s.AddTool(
		mcp.NewTool(
			"okf_ingest",
			mcp.WithDescription("Ingest an OKF v0.1 bundle (markdown with YAML frontmatter) into the knowledge graph. Returns a report with concept/link/bridge counts."),
			mcp.WithString("project_id", mcp.Required(), mcp.Description("Target project ID")),
			mcp.WithString("bundle_dir", mcp.Required(), mcp.Description("Absolute path to the OKF bundle directory")),
		),
		handleOKFIngestTool(storeMgr, dataDir),
	)

	s.AddTool(
		mcp.NewTool(
			"okf_export",
			mcp.WithDescription("Export the code graph as an OKF v0.1 bundle. Returns the number of concepts written."),
			mcp.WithString("project_id", mcp.Required(), mcp.Description("Source project ID")),
			mcp.WithString("out_dir", mcp.Required(), mcp.Description("Output bundle directory")),
			mcp.WithString("scope", mcp.Description("Export scope: file, package, or cluster (default: file)")),
		),
		handleOKFExportTool(storeMgr),
	)
}

func handleOKFIngestTool(storeMgr okf.StoreAccessor, dataDir string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, _ := req.GetArguments()["project_id"].(string)
		bundleDir, _ := req.GetArguments()["bundle_dir"].(string)
		if bundleDir == "" {
			return mcp.NewToolResultError("bundle_dir is required"), nil
		}
		if projectID == "" {
			projectID = "default"
		}

		report, err := okf.Ingest(ctx, storeMgr, dataDir, okf.IngestOptions{
			ProjectID: projectID,
			BundleDir: bundleDir,
		})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		result := fmt.Sprintf("OKF ingest complete for project %q:\n"+
			"  Concepts:     %d\n"+
			"  Links:        %d\n"+
			"  Bridges:      %d\n"+
			"  Bridge misses: %d\n"+
			"  Conformant:   %v\n"+
			"  Duration:     %s",
			projectID, report.Concepts, report.Links,
			report.Bridges, report.BridgeMiss, report.Conformant, report.Duration)
		if len(report.Errors) > 0 {
			result += fmt.Sprintf("\n  Errors (%d):", len(report.Errors))
			for _, e := range report.Errors {
				result += fmt.Sprintf("\n    - %s", e)
			}
		}
		return mcp.NewToolResultText(result), nil
	}
}

func handleOKFExportTool(storeMgr okf.StoreAccessor) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, _ := req.GetArguments()["project_id"].(string)
		outDir, _ := req.GetArguments()["out_dir"].(string)
		scopeStr, _ := req.GetArguments()["scope"].(string)
		if outDir == "" {
			return mcp.NewToolResultError("out_dir is required"), nil
		}
		if projectID == "" {
			projectID = "default"
		}
		scope := okf.ExportScope(scopeStr)
		if scope == "" {
			scope = okf.ExportFile
		}

		report, err := okf.Export(ctx, storeMgr, okf.ExportOptions{
			ProjectID:        projectID,
			OutDir:           outDir,
			Scope:            scope,
			IncludeSmells:    true,
			IncludeCitations: true,
		})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		result := fmt.Sprintf("OKF export complete for project %q:\n"+
			"  Concepts written: %d\n"+
			"  Files written:    %d\n"+
			"  Duration:         %s",
			projectID, report.ConceptsWritten, report.FilesWritten, report.Duration)
		return mcp.NewToolResultText(result), nil
	}
}
