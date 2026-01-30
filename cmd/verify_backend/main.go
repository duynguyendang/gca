package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/ingest"
	"github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/meb/store"
	"github.com/duynguyendang/gca/pkg/service"
)

// MockManager implements service.ProjectStoreManager
type MockManager struct {
	store *meb.MEBStore
}

func (m *MockManager) GetStore(id string) (*meb.MEBStore, error) {
	return m.store, nil
}

func (m *MockManager) ListProjects() ([]manager.ProjectMetadata, error) {
	return nil, nil
}

func main() {
	// 1. Setup temporary store
	dir, err := os.MkdirTemp("", "gca-verify-be-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	cfg := store.DefaultConfig(dir)
	s, err := meb.NewMEBStore(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()

	// 2. Ingest gca and gca-fe
	// Paths are absolute based on user info
	bePath := "/mnt/e/gca-hackathon/gca"
	fePath := "/mnt/e/gca-hackathon/gca-fe"

	fmt.Println("Step 1: Ingesting Backend...")
	if err := ingest.Run(s, "gca-be", bePath); err != nil {
		log.Printf("Warning: Ingestion of BE had issues: %v", err)
	}

	fmt.Println("Step 2: Ingesting Frontend...")
	if err := ingest.Run(s, "gca-fe", fePath); err != nil {
		log.Printf("Warning: Ingestion of FE had issues: %v", err)
	}

	// 3. Enhance Virtual Triples
	fmt.Println("Step 3: Enhancing Virtual Triples...")
	if err := ingest.EnhanceVirtualTriples(s); err != nil {
		log.Fatalf("Enhance failed: %v", err)
	}

	ctx := context.Background()

	mockMgr := &MockManager{store: s}
	svc := service.NewGraphService(mockMgr)

	// --- Phase 2: find_connection (BFS Validation) ---
	fmt.Println("\n--- Phase 2: find_connection (BFS Validation) ---")

	// Trace 1: FE -> BE
	fmt.Println("Trace: Inception Trace (gca-fe/App.tsx -> gca-be/pkg/service/pathfinder.go)")
	graph1, err := svc.FindShortestPath(ctx, "default", "gca-fe/App.tsx", "gca-be/pkg/service/pathfinder.go")
	if err != nil || len(graph1.Nodes) == 0 {
		fmt.Printf("  FAIL: %v\n", err)
		if len(graph1.Nodes) == 0 {
			fmt.Println("  FAIL: No path found")
		}
	} else {
		fmt.Printf("  PASS: Found path with %d nodes\n", len(graph1.Nodes))
	}

	// Trace 2: BE Model -> FE Service
	fmt.Println("Trace: Data Propagation (gca-be/pkg/meb/types.go:Fact -> gca-fe/services/geminiService.ts)")
	graph2, err := svc.FindShortestPath(ctx, "default", "gca-be/pkg/meb/types.go:Fact", "gca-fe/services/geminiService.ts")
	if err != nil || len(graph2.Nodes) == 0 {
		fmt.Printf("  FAIL: %v\n", err)
		if len(graph2.Nodes) == 0 {
			fmt.Println("  FAIL: No path found")
		}
	} else {
		fmt.Printf("  PASS: Found path with %d nodes\n", len(graph2.Nodes))
	}

	// Trace 3: FE Component -> FE Utility
	fmt.Println("Trace: Utility Discovery (gca-fe/components/TreeVisualizer.tsx -> gca-fe/utils/pathfinding.ts:bfsPath)")
	graph3, err := svc.FindShortestPath(ctx, "default", "gca-fe/components/TreeVisualizer.tsx", "gca-fe/utils/pathfinding.ts:bfsPath")
	if err != nil || len(graph3.Nodes) == 0 {
		fmt.Printf("  FAIL: %v\n", err)
		if len(graph3.Nodes) == 0 {
			fmt.Println("  FAIL: No path found")
		}
	} else {
		fmt.Printf("  PASS: Found path with %d nodes\n", len(graph3.Nodes))
	}

	// --- Phase 3: Datalog Logic (Relational Validation) ---
	fmt.Printf("\n--- Phase 3: Datalog Logic (Relational Validation) ---\n")

	// Case A: Dead Code
	qA := `
		triples(?u, "has_role", "utility"),
		NOT triples(?anyone, "calls", ?u)
	`
	resA, _ := s.Query(ctx, qA)
	if len(resA) > 0 {
		fmt.Printf("Case A (Dead Code): PASS (%d orphaned utilities)\n", len(resA))
	} else {
		fmt.Printf("Case A (Dead Code): FAIL (0 results)\n")
	}

	// Case B: API Inventory
	qB := `
		triples(?h, "has_role", "api_handler"),
		triples(?h, "exposes_model", ?m)
	`
	resB, _ := s.Query(ctx, qB)
	if len(resB) > 0 {
		fmt.Printf("Case B (API Inventory): PASS (%d handler-model pairs)\n", len(resB))
	} else {
		fmt.Printf("Case B (API Inventory): FAIL (0 results)\n")
	}

	// Case C: Package Integrity
	qC := `triples(?s, "in_package", ?p)`
	resC, _ := s.Query(ctx, qC)
	if len(resC) > 500 {
		fmt.Printf("Case C (Package Integrity): PASS (%d symbols covered)\n", len(resC))
	} else {
		fmt.Printf("Case C (Package Integrity): FAIL (Low coverage: %d)\n", len(resC))
	}

	// --- Phase 4: Performance & Quality Standards ---
	fmt.Printf("\n--- Phase 4: Performance & Quality Standards ---\n")

	// 1. Skeleton Pruning (Checked in Phase 2)
	// 2. Branching & Depth (Implemented in pathfinder.go)
	// 3. Path Labeling
	fmt.Printf("Checking Path Labeling (Trace 1)...\n")
	resL, _ := svc.FindShortestPath(ctx, "default", "gca-fe/App.tsx", "gca-be/pkg/service/pathfinder.go")
	if len(resL.Links) > 0 {
		allSemantic := true
		for _, l := range resL.Links {
			if l.Relation == "related" || l.Relation == "unknown" {
				allSemantic = false
				break
			}
		}
		if allSemantic {
			fmt.Printf("PASS: All links have semantic labels.\n")
		} else {
			fmt.Printf("FAIL: Some links have generic 'related' labels.\n")
		}
	}

	// 4. Latency
	latencyT := time.Now()
	svc.FindShortestPath(ctx, "default", "gca-fe/App.tsx", "gca-be/pkg/service/pathfinder.go")
	latency := time.Since(latencyT)
	if latency < 500*time.Millisecond {
		fmt.Printf("PASS: Latency optimal (%v)\n", latency)
	} else {
		fmt.Printf("FAIL: Latency > 500ms (%v)\n", latency)
	}
}
