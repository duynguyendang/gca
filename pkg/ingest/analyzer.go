package ingest

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/duynguyendang/gca/pkg/common"
	"github.com/duynguyendang/gca/pkg/config"
	mebpkg "github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/logger"
	"github.com/duynguyendang/meb"
)

// Local clustering types to avoid import cycle with service package.
type clusterNode struct {
	ID        string
	Weight    float64
	Neighbors map[int]float64
}
type clusterLink struct {
	Source string
	Target string
}
type clusterResult struct {
	Clusters    map[int][]string
	NodeCluster map[string]int
}

const (
	CurrentAnalyticsVersion   = "2.0"
	AnalyticsVersionPredicate = "analytics_version"
)

// Analyzer performs post-ingestion analysis on the Source Store
// and writes results (smells, centrality) to the Analytical Store.
type Analyzer struct {
	storeManager  StoreManagerInterface
	templateStore TemplateStoreInterface
}

// StoreManagerInterface defines the interface for store operations.
type StoreManagerInterface interface {
	GetSourceStore(projectID string) (*meb.MEBStore, error)
	GetAnalyticalStore(projectID string) (*meb.MEBStore, error)
}

// TemplateStoreInterface defines the interface for template operations.
type TemplateStoreInterface interface {
	GetTemplate(ctx context.Context, projectID, templateID string) (*TemplateStoreQuery, error)
	ListTemplates(ctx context.Context, projectID, category string) ([]*TemplateStoreQuery, error)
}

// TemplateStoreQuery represents a query template from the TemplateStore.
type TemplateStoreQuery struct {
	ID          string
	Body        string
	Predicate   string
	Category    string
	Severity    string
	SmellType   string
	Description string
	Parameters  []TemplateParam
}

// TemplateParam represents a parameter in a query template.
type TemplateParam struct {
	Name        string
	Type        string
	Description string
}

// NewAnalyzer creates a new Analyzer.
func NewAnalyzer(sm StoreManagerInterface, ts TemplateStoreInterface) *Analyzer {
	return &Analyzer{
		storeManager:  sm,
		templateStore: ts,
	}
}

// RunStaticAnalysis executes the full static analysis pipeline:
// 1. Clear old analytical data
// 2. Compute centrality and entry points using Datalog rules
// 3. Execute templates from TemplateStore
func (a *Analyzer) RunStaticAnalysis(ctx context.Context, projectID string) error {
	logger.Info("Starting static analysis", "project", projectID)

	if err := a.clearAnalyticalData(ctx, projectID); err != nil {
		logger.Warn("Failed to clear old analytical data", "error", err)
	}

	if err := a.computeCentrality(ctx, projectID); err != nil {
		logger.Warn("Centrality computation failed", "error", err)
	}

	if err := a.executeRulesFromTemplates(ctx, projectID); err != nil {
		logger.Warn("Template rule execution failed", "error", err)
	}

	if err := a.computeHealthScores(ctx, projectID); err != nil {
		logger.Warn("Health score computation failed", "error", err)
	}

	logger.Info("Static analysis completed", "project", projectID)
	return nil
}

// executeRulesFromTemplates loads and executes all templates from TemplateStore.
// Each template defines: query body, result predicate, and metadata.
// This is the generic rule execution engine - works for any template type.
func (a *Analyzer) executeRulesFromTemplates(ctx context.Context, projectID string) error {
	if a.templateStore == nil {
		return fmt.Errorf("template store not available")
	}

	templates, err := a.templateStore.ListTemplates(ctx, projectID, "")
	if err != nil {
		return fmt.Errorf("failed to list templates: %w", err)
	}

	if len(templates) == 0 {
		logger.Info("No templates found in TemplateStore")
		return nil
	}

	sourceStore, err := a.storeManager.GetSourceStore(projectID)
	if err != nil {
		return fmt.Errorf("failed to get source store: %w", err)
	}

	analyticalStore, err := a.storeManager.GetAnalyticalStore(projectID)
	if err != nil {
		return fmt.Errorf("failed to get analytical store: %w", err)
	}

	totalResults := 0

	for _, tmpl := range templates {
		if tmpl.Body == "" {
			continue
		}

		results, err := mebpkg.Query(ctx, sourceStore, tmpl.Body)
		if err != nil {
			logger.Warn("Template query failed", "template", tmpl.ID, "error", err)
			continue
		}

		logger.Debug("Template returned results", "template", tmpl.ID, "count", len(results))

		for _, r := range results {
			if err := a.emitFactFromTemplate(analyticalStore, r, tmpl); err != nil {
				logger.Warn("Failed to emit fact for template", "template", tmpl.ID, "error", err)
			} else {
				totalResults++
			}
		}
	}

	logger.Info("Template rule execution complete", "facts", totalResults, "templates", len(templates))
	return nil
}

// emitFactFromTemplate emits a structured fact to the analytical store.
// The fact predicate is derived from template metadata (e.g., "has_smell_type", "is_entry_point").
// Metadata fields (category, severity, etc.) are stored as separate predicates.
func (a *Analyzer) emitFactFromTemplate(store *meb.MEBStore, result map[string]any, tmpl *TemplateStoreQuery) error {
	var subject string

	if s, ok := result["File"].(string); ok {
		subject = s
	} else if s, ok := result["A"].(string); ok {
		subject = s
	} else if s, ok := result["Subject"].(string); ok {
		subject = s
	} else if s, ok := result["Source"].(string); ok {
		subject = s
	} else if s, ok := result["Target"].(string); ok {
		subject = s
	} else if s, ok := result["Symbol"].(string); ok {
		subject = s
	} else if s, ok := result["Cluster"].(string); ok {
		subject = s
	} else if s, ok := result["Concept"].(string); ok {
		subject = s
	}

	if subject == "" {
		return fmt.Errorf("no subject found in result")
	}

	predicate := tmpl.Predicate
	if predicate == "" {
		predicate = "has_" + tmpl.ID
	}

	smellType := tmpl.SmellType
	if smellType == "" {
		smellType = tmpl.ID
	}

	facts := []meb.Fact{
		{Subject: subject, Predicate: predicate, Object: smellType},
		{Subject: subject, Predicate: "has_category", Object: tmpl.Category},
		{Subject: subject, Predicate: "has_severity", Object: tmpl.Severity},
	}

	for _, fact := range facts {
		if err := store.AddFact(fact); err != nil {
			return err
		}
	}

	return nil
}

// extractString extracts a string value from a map.

// clearAnalyticalData removes old smells, hub scores, entry points, and centrality data
// from the analytical store to prevent stale data from previous ingests.
// Uses batch collection then single-pass deletion per subject to minimize DB operations.
func (a *Analyzer) clearAnalyticalData(ctx context.Context, projectID string) error {
	analyticalStore, err := a.storeManager.GetAnalyticalStore(projectID)
	if err != nil {
		return fmt.Errorf("failed to get analytical store: %w", err)
	}

	predicatesToClear := []string{
		"has_smell",
		"has_smell_type",
		"has_smell_category",
		"has_smell_severity",
		"has_hub_score",
		"is_entry_point",
		"has_centrality",
		"has_in_degree",
		"has_out_degree",
		"belongs_to_cluster",
		"has_surprise",
		"has_knowledge_gap",
		"has_category",
		"has_severity",
		"has_health_score",
		"has_health_debt",
	}

	for _, pred := range predicatesToClear {
		logger.Info("Clearing facts", "predicate", pred, "project", projectID)

		subjectsToDelete := make(map[string]bool)
		for fact, err := range analyticalStore.ScanContext(ctx, "", pred, "") {
			if err != nil {
				logger.Warn("Error scanning facts", "predicate", pred, "error", err)
				continue
			}
			subjectsToDelete[fact.Subject] = true
		}

		logger.Info("Found subjects to clear", "count", len(subjectsToDelete), "predicate", pred)

		clearedCount := 0
		for subject := range subjectsToDelete {
			if err := analyticalStore.DeleteFactsBySubject(subject); err != nil {
				logger.Warn("Failed to delete facts for subject", "subject", subject, "error", err)
			} else {
				clearedCount++
			}
		}

		if clearedCount > 0 {
			logger.Info("Cleared facts", "count", clearedCount, "predicate", pred, "project", projectID)
		}
	}

	return nil
}

// executeTemplateQueries executes smell detection templates from the TemplateStore.
func (a *Analyzer) executeTemplateQueries(ctx context.Context, projectID string) error {
	if a.templateStore == nil {
		return nil
	}

	// Get all smell templates
	templates, err := a.templateStore.ListTemplates(ctx, projectID, "smell")
	if err != nil {
		return fmt.Errorf("failed to list smell templates: %w", err)
	}

	sourceStore, err := a.storeManager.GetSourceStore(projectID)
	if err != nil {
		return fmt.Errorf("failed to get source store: %w", err)
	}

	analyticalStore, err := a.storeManager.GetAnalyticalStore(projectID)
	if err != nil {
		return fmt.Errorf("failed to get analytical store: %w", err)
	}

	for _, tmpl := range templates {
		if tmpl.Body == "" {
			continue
		}

		// Execute template query against source store
		results, err := mebpkg.Query(ctx, sourceStore, tmpl.Body)
		if err != nil {
			logger.Warn("Template query failed", "template", tmpl.ID, "error", err)
			continue
		}

		// Write results to analytical store
		for _, r := range results {
			subject := extractString(r, "Subject")
			if subject == "" {
				subject = extractString(r, "Concept")
			}
			if subject == "" {
				continue
			}

			fact := meb.Fact{
				Subject:   subject,
				Predicate: "has_smell",
				Object:    tmpl.ID + ":" + tmpl.Category,
			}
			if err := analyticalStore.AddFact(fact); err != nil {
				logger.Warn("Failed to add smell fact", "error", err)
			}
		}
	}

	return nil
}

// extractString extracts a string value from a map.
func extractString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// getAnalyticsVersion retrieves the stored analytics version from the analytical store
func (a *Analyzer) getAnalyticsVersion(analyticalStore *meb.MEBStore) string {
	ctx := context.Background()
	for item, err := range analyticalStore.ScanContext(ctx, "", AnalyticsVersionPredicate, "") {
		if err != nil {
			continue
		}
		if objStr, ok := item.Object.(string); ok {
			return objStr
		}
	}
	return ""
}

// setAnalyticsVersion stores the analytics version in the analytical store
func (a *Analyzer) setAnalyticsVersion(analyticalStore *meb.MEBStore) error {
	fact := meb.Fact{
		Subject:   "analytics",
		Predicate: AnalyticsVersionPredicate,
		Object:    CurrentAnalyticsVersion,
	}
	return analyticalStore.AddFact(fact)
}

// RunPostIngestAnalysis runs all post-ingestion analysis for a project.
// This should be called after the main ingestion is complete.
// It runs asynchronously to avoid blocking the ingestion process.
// Uses idempotent fact addition - duplicate facts won't be added if they already exist.
// Checks analytics version to skip redundant computations.
// All analysis is template-driven via TemplateStore - no hardcoded rules.
func (a *Analyzer) RunPostIngestAnalysis(ctx context.Context, projectID string) error {
	logger.Info("Starting post-ingest analysis", "project", projectID)

	analyticalStore, err := a.storeManager.GetAnalyticalStore(projectID)
	if err != nil {
		return fmt.Errorf("failed to get analytical store: %w", err)
	}

	storedVersion := a.getAnalyticsVersion(analyticalStore)
	if storedVersion == CurrentAnalyticsVersion {
		logger.Info("Analytics already up to date, skipping", "version", CurrentAnalyticsVersion)
		return nil
	}

	if err := a.clearAnalyticalData(ctx, projectID); err != nil {
		logger.Warn("Failed to clear old analytical data", "error", err)
	}

	// computeCentrality MUST run before executeRulesFromTemplates.
	// Templates like surprise.mg and knowledge_gaps.mg query facts written by
	// computeCentrality (has_in_degree, has_out_degree, belongs_to_cluster).
	if err := a.computeCentrality(ctx, projectID); err != nil {
		logger.Warn("Centrality computation failed", "error", err)
	}

	// Write okf_age_days facts so the stale smell policy (stale.mg) can fire.
	sourceStore, srcErr := a.storeManager.GetSourceStore(projectID)
	if srcErr == nil {
		if err := a.writeOKFAgeDays(ctx, sourceStore); err != nil {
			logger.Warn("OKF age days computation failed", "error", err)
		}
	}

	if err := a.executeRulesFromTemplates(ctx, projectID); err != nil {
		logger.Warn("Template rule execution failed", "error", err)
	}

	if err := a.computeHealthScores(ctx, projectID); err != nil {
		logger.Warn("Health score computation failed", "error", err)
	}

	if err := a.setAnalyticsVersion(analyticalStore); err != nil {
		logger.Warn("Failed to set analytics version", "error", err)
	}

	logger.Info("Post-ingest analysis complete", "project", projectID)
	return nil
}

// RunPostIngestAnalysisAsync runs analysis asynchronously.
func (a *Analyzer) RunPostIngestAnalysisAsync(ctx context.Context, projectID string) {
	go func() {
		if err := a.RunPostIngestAnalysis(ctx, projectID); err != nil {
			logger.Error("Async post-ingest analysis failed", "error", err)
		}
	}()
}

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

	// Write hub score facts for files with multiple callers
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
		}
	}

	// Compute entry points - files that define main, init, or have many callees
	entryResults, err := mebpkg.Query(ctx, sourceStore, common.GetNamedQuery("entry_candidates"))
	if err != nil {
		logger.Warn("Entry point query failed", "error", err)
	} else {
		for _, r := range entryResults {
			if file, ok := r["File"].(string); ok {
				fact := meb.Fact{
					Subject:   file,
					Predicate: "is_entry_point",
					Object:    "true",
				}
				if err := analyticalStore.AddFact(fact); err != nil {
					logger.Warn("Failed to add entry point", "file", file, "error", err)
				}
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

	logger.Info("Centrality analysis complete", "hub_files", len(callerCounts), "entry_points", len(entryResults))

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
		in := inDegree[sym]
		out := outDegree[sym]

		inFact := meb.Fact{Subject: sym, Predicate: "has_in_degree", Object: fmt.Sprintf("%d", in)}
		if err := analyticalStore.AddFact(inFact); err != nil {
			logger.Warn("Failed to add in_degree fact", "symbol", sym, "error", err)
		} else {
			factCount++
		}

		outFact := meb.Fact{Subject: sym, Predicate: "has_out_degree", Object: fmt.Sprintf("%d", out)}
		if err := analyticalStore.AddFact(outFact); err != nil {
			logger.Warn("Failed to add out_degree fact", "symbol", sym, "error", err)
		} else {
			factCount++
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

// detectSmells runs smell detection queries and writes results to Analytical Store.
// Smell templates are loaded from the TemplateStore (backed by policies/smells/*.mg).
// This makes smell detection fully Datalog-driven - adding new smells requires only .mg files.
func (a *Analyzer) detectSmells(ctx context.Context, projectID string) error {
	sourceStore, err := a.storeManager.GetSourceStore(projectID)
	if err != nil {
		return fmt.Errorf("failed to get source store: %w", err)
	}

	analyticalStore, err := a.storeManager.GetAnalyticalStore(projectID)
	if err != nil {
		return fmt.Errorf("failed to get analytical store: %w", err)
	}

	if a.templateStore == nil {
		return fmt.Errorf("template store not available for smell detection")
	}

	templates, err := a.templateStore.ListTemplates(ctx, projectID, "smell")
	if err != nil {
		return fmt.Errorf("failed to list smell templates: %w", err)
	}

	smellResults := 0

	for _, tmpl := range templates {
		if tmpl.Body == "" {
			logger.Warn("Template has empty body", "template", tmpl.ID)
			continue
		}

		results, err := mebpkg.Query(ctx, sourceStore, tmpl.Body)
		if err != nil {
			logger.Warn("Smell query failed", "template", tmpl.ID, "error", err)
			continue
		}

		for _, r := range results {
			if err := a.emitStructuredFact(analyticalStore, r, tmpl.ID, tmpl.Category, tmpl.Severity); err != nil {
				logger.Warn("Failed to emit structured fact", "error", err)
			} else {
				smellResults++
			}
		}
	}

	logger.Info("Smell detection complete", "smells", smellResults)
	return nil
}

// emitStructuredFact emits a structured fact triple to the analytical store.
// The predicate is derived from the template ID (e.g., "smell_circular_direct").
// Value fields are stored as separate predicates.
func (a *Analyzer) emitStructuredFact(store *meb.MEBStore, result map[string]any, templateID, category, severity string) error {
	var subject string

	if s, ok := result["File"].(string); ok {
		subject = s
	} else if s, ok := result["A"].(string); ok {
		subject = s
	} else if s, ok := result["Subject"].(string); ok {
		subject = s
	} else if s, ok := result["Concept"].(string); ok {
		subject = s
	}

	if subject == "" {
		return fmt.Errorf("no subject found in result")
	}

	facts := []meb.Fact{
		{Subject: subject, Predicate: "has_smell_type", Object: templateID},
		{Subject: subject, Predicate: "has_smell_category", Object: category},
		{Subject: subject, Predicate: "has_smell_severity", Object: severity},
	}

	for _, fact := range facts {
		if err := store.AddFact(fact); err != nil {
			return err
		}
	}

	return nil
}

// computeHealthScores executes scoring.mg rules against the Analytical Store
// and writes pre-computed health scores and debt facts.
func (a *Analyzer) computeHealthScores(ctx context.Context, projectID string) error {
	analyticalStore, err := a.storeManager.GetAnalyticalStore(projectID)
	if err != nil {
		return fmt.Errorf("failed to get analytical store: %w", err)
	}

	healthDebtQuery := `health_debt_with_hub(File, Total)`
	results, err := mebpkg.Query(ctx, analyticalStore, healthDebtQuery)
	if err != nil {
		return fmt.Errorf("health_debt query failed: %w", err)
	}

	debtCount := 0
	for _, r := range results {
		file, ok := r["File"].(string)
		if !ok || file == "" {
			continue
		}
		total, ok := r["Total"].(float64)
		if !ok {
			continue
		}
		fact := meb.Fact{
			Subject:   file,
			Predicate: config.PredicateHasHealthDebt,
			Object:    fmt.Sprintf("%.0f", total),
		}
		if err := analyticalStore.AddFact(fact); err != nil {
			logger.Warn("Failed to add health_debt fact", "file", file, "error", err)
		} else {
			debtCount++
		}
	}

	healthScoreQuery := `health_score(File, Score)`
	scoreResults, err := mebpkg.Query(ctx, analyticalStore, healthScoreQuery)
	if err != nil {
		return fmt.Errorf("health_score query failed: %w", err)
	}

	scoreCount := 0
	for _, r := range scoreResults {
		file, ok := r["File"].(string)
		if !ok || file == "" {
			continue
		}
		score, ok := r["Score"].(float64)
		if !ok {
			continue
		}
		fact := meb.Fact{
			Subject:   file,
			Predicate: config.PredicateHasHealthScore,
			Object:    fmt.Sprintf("%.0f", score),
		}
		if err := analyticalStore.AddFact(fact); err != nil {
			logger.Warn("Failed to add health_score fact", "file", file, "error", err)
		} else {
			scoreCount++
		}
	}

	logger.Info("Health scores computed", "debt_facts", debtCount, "score_facts", scoreCount)
	return nil
}
