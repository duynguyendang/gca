package mcp

import (
	"context"
	"fmt"

	"github.com/duynguyendang/gca/pkg/okf"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// AddOKFTools registers OKF export tool on the MCP server.
// OKF ingest is handled by the standard gca ingest pipeline.
func AddOKFTools(s *server.MCPServer, storeMgr okf.StoreAccessor, dataDir string) {
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
