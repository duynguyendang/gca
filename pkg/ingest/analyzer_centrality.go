package ingest

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/duynguyendang/gca/pkg/common"
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/logger"
	mebpkg "github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/meb"
)

// computeCentrality calculates in/out degree for files and functions,
// then writes centrality facts to the Analytical Store.
func (a *Analyzer) computeCentrality(ctx context.Context, projectID string) error {
	sourceStore, err := a.storeManager.GetSourceStore(projectID)
	if err != nil {
		return fmt.Errorf("failed to get source store: %w", err)
	}

	analyticalStore, err := a.storeManager.GetAnalyticalStore(projectID)
	if err != nil {
		return fmt.Errorf("failed to get analytical store: %w", err)
	}

	// Compute hub scores - files that are called by many others
	hubResults, err := mebpkg.Query(ctx, sourceStore, common.GetNamedQuery("hub_candidates"))
	if err != nil {
		return fmt.Errorf("hub query failed: %w", err)
	}

	// Count callers per file
	callerCounts := make(map[string]int)
	for _, r := range hubResults {
		if file, ok := r["File"].(string); ok {
			callerCounts[file]++
		}
	}

	// Write hub score facts for files with multiple callers.
	// Written to BOTH stores: the Analytical Store is canonical, but templates
	// (smell_hub) execute against the Source Store.
	for file, count := range callerCounts {
		if count > config.HubClassificationThreshold {
			fact := meb.Fact{
				Subject:   file,
				Predicate: "has_hub_score",
				Object:    fmt.Sprintf("%d", count),
			}
			if err := analyticalStore.AddFact(fact); err != nil {
				logger.Warn("Failed to add hub score", "file", file, "error", err)
			}
			if err := sourceStore.AddFact(fact); err != nil {
				logger.Warn("Failed to add hub score to source store", "file", file, "error", err)
			}
		}
	}

	// Compute entry points - files that define main/init. Done in Go because the
	// original `or(contains(Symbol, "main"), contains(Symbol, "init"))` named
	// query uses or() which the meb query layer rejects loudly (contract.md §5).
	entryCount := 0
	for fact := range sourceStore.ScanContext(ctx, "", config.PredicateDefines, "") {
		fileID := fact.Subject
		if strings.Contains(fileID, ":") {
			continue
		}
		symID, ok := fact.Object.(string)
		if !ok {
			continue
		}
		name := common.ExtractSymbolName(symID)
		if name == "main" || name == "init" {
			entryCount++
			fact := meb.Fact{
				Subject:   fileID,
				Predicate: "is_entry_point",
				Object:    "true",
			}
			if err := analyticalStore.AddFact(fact); err != nil {
				logger.Warn("Failed to add entry point", "file", fileID, "error", err)
			}
		}
	}

	// Compute centrality for symbols (functions/methods)
	symbolCentralityQuery := common.GetNamedQuery("symbol_calls")
	symbolResults, err := mebpkg.Query(ctx, sourceStore, symbolCentralityQuery)
	if err != nil {
		logger.Warn("Symbol centrality query failed", "error", err)
	} else {
		// Count calls per symbol
		symbolCalls := make(map[string]int)
		for _, r := range symbolResults {
			if symbol, ok := r["Symbol"].(string); ok {
				symbolCalls[symbol]++
			}
		}

		for symbol, count := range symbolCalls {
			if count > config.CentralityHighConnectThreshold {
				fact := meb.Fact{
					Subject:   symbol,
					Predicate: "has_centrality",
					Object:    fmt.Sprintf("%d", count),
				}
				if err := analyticalStore.AddFact(fact); err != nil {
					logger.Warn("Failed to add centrality", "symbol", symbol, "error", err)
				}
			}
		}
	}

	logger.Info("Centrality analysis complete", "hub_files", len(callerCounts), "entry_points", entryCount)

	// Write degree facts for ALL symbols (not just high-centrality ones)
	// This enables Datalog queries for surprise scoring and knowledge gap analysis
	if err := a.writeDegreeFacts(ctx, sourceStore, analyticalStore); err != nil {
		logger.Warn("Failed to write degree facts", "error", err)
	}

	// Write community facts (belongs_to_cluster)
	if err := a.writeCommunityFacts(ctx, sourceStore, analyticalStore); err != nil {
		logger.Warn("Failed to write community facts", "error", err)
	}

	return nil
}

// writeDegreeFacts computes in/out degree for all symbols and writes as facts.
// Facts are written to BOTH stores: the Analytical Store is the canonical home,
// but executeRulesFromTemplates runs template bodies against the Source Store, and
// templates like smell_hub, gap_isolated, surprise_peripheral_hub and
// okf_hub_anomaly query has_in_degree/has_out_degree (see analyzer_analysis.go).
func (a *Analyzer) writeDegreeFacts(ctx context.Context, sourceStore, analyticalStore *meb.MEBStore) error {
	inDegree := make(map[string]int)
	outDegree := make(map[string]int)

	for fact := range sourceStore.ScanContext(ctx, "", config.PredicateCalls, "") {
		if obj, ok := fact.Object.(string); ok {
			inDegree[obj]++
		}
		outDegree[fact.Subject]++
	}

	for fact := range sourceStore.ScanContext(ctx, "", config.PredicateImports, "") {
		if obj, ok := fact.Object.(string); ok {
			inDegree[obj]++
		}
		outDegree[fact.Subject]++
	}

	// OKF extension: include okf_link edges (Source Store) and bridges_to edges
	// (Analytical Store) in the degree calculation. This ensures OKF concepts get
	// has_in_degree/has_out_degree facts so they participate in the smell pipeline
	// (okf_hub_anomaly, okf_orphan_concept via isolated-node check) and Leiden
	// community detection. See docs/designs/okf-support.md "Analyzer Integration".
	for fact := range sourceStore.ScanContext(ctx, "", "okf_link", "") {
		if obj, ok := fact.Object.(string); ok {
			inDegree[obj]++
		}
		outDegree[fact.Subject]++
	}
	for fact := range analyticalStore.ScanContext(ctx, "", "bridges_to", "") {
		if obj, ok := fact.Object.(string); ok {
			inDegree[obj]++
		}
		outDegree[fact.Subject]++
	}

	allSymbols := make(map[string]bool)
	for sym := range inDegree {
		allSymbols[sym] = true
	}
	for sym := range outDegree {
		allSymbols[sym] = true
	}

	factCount := 0
	for sym := range allSymbols {
		if sym == "" {
			// Some call facts carry empty subject/object strings (garbage rows
			// from deleted symbols). Skip them instead of erroring on AddFact.
			continue
		}
		in := inDegree[sym]
		out := outDegree[sym]

		inFact := meb.Fact{Subject: sym, Predicate: "has_in_degree", Object: fmt.Sprintf("%d", in)}
		if err := analyticalStore.AddFact(inFact); err != nil {
			logger.Warn("Failed to add in_degree fact", "symbol", sym, "error", err)
		} else {
			factCount++
		}
		if err := sourceStore.AddFact(inFact); err != nil {
			logger.Warn("Failed to add in_degree fact to source store", "symbol", sym, "error", err)
		}

		outFact := meb.Fact{Subject: sym, Predicate: "has_out_degree", Object: fmt.Sprintf("%d", out)}
		if err := analyticalStore.AddFact(outFact); err != nil {
			logger.Warn("Failed to add out_degree fact", "symbol", sym, "error", err)
		} else {
			factCount++
		}
		if err := sourceStore.AddFact(outFact); err != nil {
			logger.Warn("Failed to add out_degree fact to source store", "symbol", sym, "error", err)
		}
	}

	logger.Debug("Degree facts written", "facts", factCount, "symbols", len(allSymbols))
	return nil
}

// writeOKFAgeDays computes okf_age_days for each OKF concept based on its okf_timestamp.
// This enables the stale smell policy (stale.mg) to detect concepts older than 90 days.
func (a *Analyzer) writeOKFAgeDays(ctx context.Context, sourceStore *meb.MEBStore) error {
	now := time.Now()
	factCount := 0

	for fact := range sourceStore.ScanContext(ctx, "", "okf_timestamp", "") {
		tsStr, ok := fact.Object.(string)
		if !ok || tsStr == "" {
			continue
		}

		// Try ISO 8601 formats: full datetime, date-only, or with timezone
		var ts time.Time
		var parseErr error
		for _, layout := range []string{
			time.RFC3339,
			"2006-01-02T15:04:05Z",
			"2006-01-02",
			time.RFC3339Nano,
		} {
			ts, parseErr = time.Parse(layout, tsStr)
			if parseErr == nil {
				break
			}
		}
		if parseErr != nil {
			continue
		}

		days := int(now.Sub(ts).Hours() / 24)
		if days < 0 {
			days = 0
		}

		ageFact := meb.Fact{
			Subject:   fact.Subject,
			Predicate: "okf_age_days",
			Object:    fmt.Sprintf("%d", days),
		}
		if err := sourceStore.AddFact(ageFact); err != nil {
			logger.Warn("Failed to add okf_age_days fact", "subject", fact.Subject, "error", err)
		} else {
			factCount++
		}
	}

	logger.Debug("OKF age days facts written", "facts", factCount)
	return nil
}

// writeFileStats computes per-file import/define counts and flags god files
// ("excessive" imports/defines). Thresholds mirror the design doc
// (docs/designs/architecture-smell-detection.md): >30 defines, >50 imports.
// Facts are written to the Source Store so the smell_god_file template fires.
func (a *Analyzer) writeFileStats(ctx context.Context, sourceStore *meb.MEBStore) error {
	importCount := make(map[string]int)
	defineCount := make(map[string]int)

	for fact := range sourceStore.ScanContext(ctx, "", config.PredicateImports, "") {
		if !strings.Contains(fact.Subject, ":") {
			importCount[fact.Subject]++
		}
	}
	for fact := range sourceStore.ScanContext(ctx, "", config.PredicateDefines, "") {
		if !strings.Contains(fact.Subject, ":") {
			defineCount[fact.Subject]++
		}
	}

	factCount := 0
	for file, count := range importCount {
		if count > config.GodFileImportThreshold {
			f := meb.Fact{Subject: file, Predicate: "has_import_count", Object: "excessive"}
			if err := sourceStore.AddFact(f); err != nil {
				logger.Warn("Failed to add has_import_count", "file", file, "error", err)
			} else {
				factCount++
			}
		}
	}
	for file, count := range defineCount {
		if count > config.GodFileDefineThreshold {
			f := meb.Fact{Subject: file, Predicate: "has_define_count", Object: "excessive"}
			if err := sourceStore.AddFact(f); err != nil {
				logger.Warn("Failed to add has_define_count", "file", file, "error", err)
			} else {
				factCount++
			}
		}
	}

	logger.Debug("File stats written", "facts", factCount, "god_imports", len(importCount), "god_defines", len(defineCount))
	return nil
}

// writeThinCommunities flags clusters with fewer than minMembers nodes as thin
// communities (knowledge gap). Replaces the gap_thin_community template, which
// used `Count = 1` (aggregation the meb query layer cannot express).
func (a *Analyzer) writeThinCommunities(ctx context.Context, sourceStore, analyticalStore *meb.MEBStore) error {
	members := make(map[string]int)
	for fact := range sourceStore.ScanContext(ctx, "", "belongs_to_cluster", "") {
		if obj, ok := fact.Object.(string); ok {
			members[obj]++
		}
	}

	const minMembers = 3
	factCount := 0
	for cluster, count := range members {
		if count < minMembers {
			// Emit on each member node so consumers can group by cluster.
			for fact := range sourceStore.ScanContext(ctx, "", "belongs_to_cluster", cluster) {
				f := meb.Fact{Subject: fact.Subject, Predicate: "has_knowledge_gap", Object: "thin"}
				if err := analyticalStore.AddFact(f); err != nil {
					logger.Warn("Failed to add thin community", "node", fact.Subject, "error", err)
				} else {
					factCount++
				}
			}
		}
	}

	logger.Debug("Thin communities written", "facts", factCount, "clusters", len(members))
	return nil
}

// computeSurpriseScores scores call edges by how "surprising" the coupling is.
// Replaces the surprise_score/surprise_top/surprise_hotspot templates, which
// used `Score = 1` / `Count > 0` (aggregation the meb query layer cannot express).
// Each surprise flag adds to an edge score; per-file totals are also recorded.
func (a *Analyzer) computeSurpriseScores(ctx context.Context, projectID string) error {
	sourceStore, err := a.storeManager.GetSourceStore(projectID)
	if err != nil {
		return err
	}
	analyticalStore, err := a.storeManager.GetAnalyticalStore(projectID)
	if err != nil {
		return err
	}

	clusterOf := make(map[string]string)
	for fact := range sourceStore.ScanContext(ctx, "", "belongs_to_cluster", "") {
		if obj, ok := fact.Object.(string); ok {
			clusterOf[fact.Subject] = obj
		}
	}
	langOf := make(map[string]string)
	for fact := range sourceStore.ScanContext(ctx, "", config.PredicateHasLanguage, "") {
		if obj, ok := fact.Object.(string); ok {
			langOf[fact.Subject] = obj
		}
	}
	isTest := make(map[string]bool)
	for fact := range sourceStore.ScanContext(ctx, "", "is_test_symbol", "true") {
		isTest[fact.Subject] = true
	}
	outDegree := make(map[string]int)
	for fact := range sourceStore.ScanContext(ctx, "", "has_out_degree", "") {
		if obj, ok := fact.Object.(string); ok {
			var n int
			if _, err := fmt.Sscanf(obj, "%d", &n); err == nil {
				outDegree[fact.Subject] = n
			}
		}
	}

	edgeScore := make(map[string]map[string]int) // subject -> target -> score
	fileScore := make(map[string]int)            // file (in_file of subject) -> total score

	for fact := range sourceStore.ScanContext(ctx, "", config.PredicateCalls, "") {
		target, ok := fact.Object.(string)
		if !ok || target == "" {
			continue
		}
		src := fact.Subject
		score := 0
		if clusterOf[src] != "" && clusterOf[target] != "" && clusterOf[src] != clusterOf[target] {
			score++
		}
		if langOf[src] != "" && langOf[target] != "" && langOf[src] != langOf[target] {
			score++
		}
		if !isTest[src] && isTest[target] {
			score += 2
		}
		if outDegree[src] == 0 && outDegree[target] > 0 {
			score++
		}
		if score == 0 {
			continue
		}
		if edgeScore[src] == nil {
			edgeScore[src] = make(map[string]int)
		}
		edgeScore[src][target] = score

		if edgeFact := (meb.Fact{
			Subject:   src,
			Predicate: "has_surprise_score",
			Object:    fmt.Sprintf("%d", score),
		}); analyticalStore.AddFact(edgeFact) != nil {
			// best-effort; individual failure not fatal
		}

		// Attribute to the file containing src (surprise_hotspot aggregation).
		for ff := range sourceStore.ScanContext(ctx, "", "in_file", src) {
			fileScore[ff.Subject] += score
		}
	}

	// Emit per-file hotspot facts (surprise_hotspot replacement).
	for file, total := range fileScore {
		if f := (meb.Fact{Subject: file, Predicate: "has_surprise", Object: fmt.Sprintf("hotspot_%d", total)}); analyticalStore.AddFact(f) != nil {
			// best-effort
		}
	}

	logger.Debug("Surprise scores computed", "edges", len(edgeScore), "files", len(fileScore))
	return nil
}

// writeCommunityFacts runs Leiden community detection and writes belongs_to_cluster facts.
func (a *Analyzer) writeCommunityFacts(ctx context.Context, sourceStore, analyticalStore *meb.MEBStore) error {
	var nodes []clusterNode
	var links []clusterLink

	seenNodes := make(map[string]bool)
	nodeMap := make(map[string]int)

	for fact := range sourceStore.ScanContext(ctx, "", config.PredicateCalls, "") {
		if !seenNodes[fact.Subject] {
			seenNodes[fact.Subject] = true
			nodeMap[fact.Subject] = len(nodes)
			nodes = append(nodes, clusterNode{ID: fact.Subject, Weight: 1.0, Neighbors: make(map[int]float64)})
		}
		if obj, ok := fact.Object.(string); ok && !seenNodes[obj] {
			seenNodes[obj] = true
			nodeMap[obj] = len(nodes)
			nodes = append(nodes, clusterNode{ID: obj, Weight: 1.0, Neighbors: make(map[int]float64)})
		}
		if obj, ok := fact.Object.(string); ok {
			srcIdx := nodeMap[fact.Subject]
			tgtIdx := nodeMap[obj]
			nodes[srcIdx].Neighbors[tgtIdx]++
			nodes[tgtIdx].Neighbors[srcIdx]++
			links = append(links, clusterLink{Source: fact.Subject, Target: obj})
		}
	}

	if len(nodes) == 0 {
		logger.Debug("No nodes for community detection")
		return nil
	}

	// Guard against empty graph (no edges)
	result := detectCommunitiesLeidenLocal(nodes)

	factCount := 0
	for nodeID, clusterID := range result.NodeCluster {
		clusterFact := meb.Fact{
			Subject:   nodeID,
			Predicate: "belongs_to_cluster",
			Object:    fmt.Sprintf("cluster_%d", clusterID),
		}
		if err := sourceStore.AddFact(clusterFact); err != nil {
			logger.Warn("Failed to add cluster fact to source store", "node", nodeID, "error", err)
		} else {
			factCount++
		}
		if err := analyticalStore.AddFact(clusterFact); err != nil {
			logger.Warn("Failed to add cluster fact to analytical store", "node", nodeID, "error", err)
		}
	}

	logger.Debug("Community facts written", "facts", factCount, "clusters", len(result.Clusters))
	return nil
}

// detectCommunitiesLeidenLocal is a local implementation of Leiden algorithm.
func detectCommunitiesLeidenLocal(nodes []clusterNode) *clusterResult {
	if len(nodes) == 0 {
		return &clusterResult{Clusters: map[int][]string{}, NodeCluster: map[string]int{}}
	}

	totalWeight := 0.0
	for i := range nodes {
		for _, w := range nodes[i].Neighbors {
			totalWeight += w
		}
	}
	totalWeight /= 2

	// Guard against empty graph (no edges) - return singleton communities
	if totalWeight == 0 {
		result := &clusterResult{
			Clusters:    map[int][]string{},
			NodeCluster: map[string]int{},
		}
		for i, n := range nodes {
			result.Clusters[i] = []string{n.ID}
			result.NodeCluster[n.ID] = i
		}
		return result
	}

	partition := make([]int, len(nodes))
	for i := range partition {
		partition[i] = i
	}

	commWeight := make([]float64, len(nodes))
	for i := range nodes {
		commWeight[i] = nodes[i].Weight
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	improved := true
	resolution := 0.1

	for pass := 0; pass < 10 && improved; pass++ {
		improved = false
		order := make([]int, len(nodes))
		for i := range order {
			order[i] = i
		}
		rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })

		for _, uIdx := range order {
			u := nodes[uIdx]
			oldComm := partition[uIdx]

			neighborComms := make(map[int]float64)
			for vIdx, w := range u.Neighbors {
				neighborComms[partition[vIdx]] += w
			}
			neighborComms[oldComm] += 0

			bestComm := oldComm
			k_i := u.Weight
			factor := k_i / (2 * totalWeight)

			w_in_old := neighborComms[oldComm]
			tot_old := commWeight[oldComm] - k_i
			gain_old := w_in_old - (resolution * tot_old * factor)

			for c, w_in := range neighborComms {
				if c == oldComm {
					continue
				}
				tot := commWeight[c]
				gain := w_in - (resolution * tot * factor)
				if gain > gain_old+1e-9 {
					gain_old = gain
					bestComm = c
					improved = true
				}
			}

			if bestComm != oldComm {
				commWeight[oldComm] -= k_i
				commWeight[bestComm] += k_i
				partition[uIdx] = bestComm
			}
		}
	}

	finalClusters := make(map[int][]string)
	finalNodeMap := make(map[string]int)
	newCommMap := make(map[int]int)
	nextID := 0

	for i, commID := range partition {
		realCommID, exists := newCommMap[commID]
		if !exists {
			realCommID = nextID
			newCommMap[commID] = realCommID
			nextID++
		}
		finalClusters[realCommID] = append(finalClusters[realCommID], nodes[i].ID)
		finalNodeMap[nodes[i].ID] = realCommID
	}

	return &clusterResult{Clusters: finalClusters, NodeCluster: finalNodeMap}
}
