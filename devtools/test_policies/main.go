package main

import (
	"context"
	"fmt"
	"os"

	manglesdk "github.com/duynguyendang/manglekit/sdk"
)

func main() {
	ctx := context.Background()
	client, err := manglesdk.NewClient(ctx)
	if err != nil {
		fmt.Printf("Failed to create manglekit client: %v\n", err)
		os.Exit(1)
	}
	defer client.Shutdown(ctx)

	engine := client.Engine()
	files := []string{
		"policies/queries.mg",
		"policies/security_agent.mg",
		"policies/quality_agent.mg",
		"policies/performance_agent.mg",
		"policies/logic_consistency_agent.mg",
		"policies/impact_agent.mg",
		"policies/intent_templates.mg",
		"policies/smells/_decl.mg",
		"policies/smells/circular.mg",
		"policies/smells/god_file.mg",
		"policies/smells/hub.mg",
		"policies/smells/layer.mg",
		"policies/smells/security.mg",
		"policies/smells/surprise.mg",
		"policies/smells/knowledge_gaps.mg",
		"policies/smells/scoring.mg",
		"policies/memory/promotion.mg",
	}

	passed := 0
	failed := 0

	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			fmt.Printf("❌ %s: cannot read file\n", f)
			failed++
			continue
		}

		err = engine.LoadPolicy(ctx, string(content))
		if err != nil {
			fmt.Printf("❌ %s: %v\n", f, err)
			failed++
		} else {
			fmt.Printf("✅ %s: OK\n", f)
			passed++
		}
	}

	fmt.Printf("\n=== Summary: %d passed, %d failed ===\n", passed, failed)
}