package ooda

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/meb"
)

// scanFactsWithTopicID scans facts using a specific TopicID without modifying the store's default TopicID.
// Uses ScanInTopicContext for thread-safe operation within a specific topic partition.
func scanFactsWithTopicID(ctx context.Context, store *meb.MEBStore, topicID uint32, subj, pred, obj string) <-chan struct {
	Fact meb.Fact
	Err  error
} {
	ch := make(chan struct {
		Fact meb.Fact
		Err  error
	}, 1)

	go func() {
		defer close(ch)
		// Use ScanInTopicContext for thread-safe scanning within a specific TopicID
		// This avoids the race condition of SetTopicID/TopicID on shared store state
		for fact, err := range store.ScanInTopicContext(ctx, topicID, subj, pred, obj) {
			ch <- struct {
				Fact meb.Fact
				Err  error
			}{Fact: fact, Err: err}
		}
	}()

	return ch
}

type StoreManager interface {
	GetStore(projectID string) (*meb.MEBStore, error)
}

type GraphObserver struct {
	storeManager StoreManager
}

func NewGraphObserver(storeManager StoreManager) *GraphObserver {
	return &GraphObserver{
		storeManager: storeManager,
	}
}

func (o *GraphObserver) Observe(ctx context.Context, frame *GCAFrame) error {
	frame.Phase = PhaseObserve

	input := frame.Input
	task := frame.Task

	if task == "" {
		task = o.classifyIntent(input)
		frame.Task = task
	}

	frame.Intent = IntentStr(task)

	frame.Context = append(frame.Context, Atom{
		Predicate: "raw_input",
		Subject:   frame.ID.String(),
		Object:    input,
		Weight:    1.0,
	})

	frame.Context = append(frame.Context, Atom{
		Predicate: "task_classified",
		Subject:   frame.ID.String(),
		Object:    string(task),
		Weight:    1.0,
	})

	frame.Context = append(frame.Context, Atom{
		Predicate: "project_id",
		Subject:   frame.ID.String(),
		Object:    frame.ProjectID,
		Weight:    1.0,
	})

	symbols := ExtractPotentialSymbols(input)
	frame.Context = append(frame.Context, Atom{
		Predicate: "potential_symbols",
		Subject:   frame.ID.String(),
		Object:    strings.Join(symbols, ","),
		Weight:    0.7,
	})

	if frame.SymbolID != "" {
		frame.Context = append(frame.Context, Atom{
			Predicate: "target_symbol",
			Subject:   frame.ID.String(),
			Object:    frame.SymbolID,
			Weight:    1.0,
		})
	}

	return nil
}

func (o *GraphObserver) classifyIntent(input string) GCATask {
	inputLower := strings.ToLower(input)

	// Enhanced patterns with confidence weighting
	type intentPattern struct {
		task    GCATask
		pattern string
		weight  int
	}

	patterns := []intentPattern{
		// High-priority architectural analysis
		{task: TaskInsight, pattern: `analyze.*component|architectural.*role|design.*pattern|structure.*overview`, weight: 10},
		{task: TaskInsight, pattern: `dependency.*graph|module.*depend|coupling`, weight: 8},

		// Flow and path analysis
		{task: TaskNarrative, pattern: `explain.*flow|trace.*path|call.*chain|data.*flow|execution.*path`, weight: 10},
		{task: TaskNarrative, pattern: `how.*work|what.*happen|follow.*call|sequence.*diagram`, weight: 7},

		// Symbol resolution
		{task: TaskResolveSymbol, pattern: `find.*handler|resolve.*symbol|which.*function|where.*defin|locate.*method`, weight: 10},
		{task: TaskResolveSymbol, pattern: `implementation.*of|called.*by|calls.*to`, weight: 8},

		// API/Endpoint discovery
		{task: TaskPathEndpoints, pattern: `endpoints.*path|api.*endpoints|route.*list|http.*handler`, weight: 10},
		{task: TaskPathEndpoints, pattern: `api.*surface|exposed.*method|public.*interface`, weight: 7},

		// Datalog queries
		{task: TaskDatalog, pattern: `query.*datalog|datalog.*query|prolog`, weight: 10},

		// Graph pruning
		{task: TaskPrune, pattern: `top.*nodes|key.*components|prune|simplif`, weight: 8},

		// File/file summary
		{task: TaskSummary, pattern: `summarize.*file|file.*summary|explain.*code|code.*review`, weight: 8},
		{task: TaskSummary, pattern: `what.*does|purpose.*of|functionality`, weight: 5},

		// Smart search
		{task: TaskSmartSearchAnalysis, pattern: `smart.*search.*analysis|search.*results.*context|find.*similar`, weight: 10},

		// Multi-file
		{task: TaskMultiFileSummary, pattern: `multiple.*files|file.*list|bulk.*analyze|compare.*file`, weight: 8},

		// NEW: Refactoring intent
		{task: TaskRefactor, pattern: `refactor|reorganiz|extract.*method|clean.*code|improve.*code`, weight: 9},
		{task: TaskRefactor, pattern: `technical.*debt|code.*smell|duplicate`, weight: 8},

		// NEW: Test generation intent
		{task: TaskTestGeneration, pattern: `test.*generat|write.*test|unit.*test|test.*case|coverage`, weight: 10},
		{task: TaskTestGeneration, pattern: `mock.*stub|test.*behavior|verify.*function`, weight: 7},

		// NEW: Security audit intent
		{task: TaskSecurityAudit, pattern: `security|vulnerab|audit|permission|auth|access.*control|injection`, weight: 10},
		{task: TaskSecurityAudit, pattern: `sanitiz|validate.*input|secure.*coding`, weight: 8},

		// NEW: Performance intent
		{task: TaskPerformance, pattern: `performance|bottleneck|slow.*method|optimi|memory.*leak`, weight: 10},
		{task: TaskPerformance, pattern: `complexity|big.*o|algorithmic`, weight: 8},
	}

	// Find best matching intent
	bestMatch := TaskChat
	bestWeight := 0

	for _, p := range patterns {
		if matched, _ := regexp.MatchString(p.pattern, inputLower); matched {
			if p.weight > bestWeight {
				bestWeight = p.weight
				bestMatch = p.task
			}
		}
	}

	// Question detection as fallback (when no pattern matched)
	if bestWeight == 0 {
		questionWords := []string{"what", "how", "why", "where", "which", "when", "who", "explain", "describe"}
		for _, q := range questionWords {
			if strings.Contains(inputLower, q) {
				// Determine specific question type
				if strings.Contains(inputLower, "where") || strings.Contains(inputLower, "find") {
					return TaskResolveSymbol
				}
				if strings.Contains(inputLower, "how") && strings.Contains(inputLower, "work") {
					return TaskNarrative
				}
				return TaskChat
			}
		}
	}

	return bestMatch
}

type GraphOrienter struct {
	storeManager        StoreManager
	maxSymbols          int
	maxResults          int
	attentionThreshold   float64
	maxAttentionSymbols int
	stickyOnlyMode      bool
}

func NewGraphOrienter(storeManager StoreManager) *GraphOrienter {
	return &GraphOrienter{
		storeManager:        storeManager,
		maxSymbols:          config.MaxAttentionSymbols,
		maxResults:          50,
		attentionThreshold:  config.VirtualAttentionThreshold,
		maxAttentionSymbols: config.MaxAttentionSymbols,
		stickyOnlyMode:      config.StickyOnlyMode,
	}
}

func (o *GraphOrienter) Orient(ctx context.Context, frame *GCAFrame) error {
	frame.Phase = PhaseOrient

	store, err := o.storeManager.GetStore(frame.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to get store for project %s: %w", frame.ProjectID, err)
	}

	symbols := ExtractPotentialSymbols(frame.Input)
	seen := make(map[string]bool)
	count := 0

	fileMap := o.buildFileMap(ctx, store, frame.ProjectID)

	centrality := o.computeDegreeCentrality(ctx, store, symbols, frame.ProjectID)
	sortByCentralityDesc(symbols, centrality)

	for _, symbol := range symbols {
		if count >= o.maxAttentionSymbols {
			break
		}
		if seen[symbol] {
			continue
		}
		// Virtual attention filter: skip symbols below centrality threshold
		if centrality[symbol] < o.attentionThreshold {
			continue
		}
		seen[symbol] = true

		inbound := o.scanWithLimit(ctx, store, symbol, "calls", o.maxResults, frame.ProjectID)
		outbound := o.scanWithLimitOutgoing(ctx, store, symbol, o.maxResults, frame.ProjectID)
		defines := o.scanWithLimitOutgoing(ctx, store, symbol, o.maxResults, frame.ProjectID)

		if len(inbound) > 0 || len(outbound) > 0 || len(defines) > 0 {
			count++

			filePath := fileMap[symbol]
			centralityScore := centrality[symbol]

			frame.Context = append(frame.Context, Atom{
				Predicate: "symbol_context",
				Subject:   symbol,
				Object:    fmt.Sprintf("in:%d out:%d def:%d file:%s centrality:%.2f", len(inbound), len(outbound), len(defines), filePath, centralityScore),
				Weight:    0.8 + centralityScore*0.2,
			})

			if len(inbound) > 0 && len(inbound) <= 5 {
				var callers []string
				for f := range inbound {
					callers = append(callers, f)
				}
				frame.Context = append(frame.Context, Atom{
					Predicate: "callers",
					Subject:   symbol,
					Object:    strings.Join(callers, ","),
					Weight:    0.6,
				})
			}
		}
	}

	frame.Context = append(frame.Context, Atom{
		Predicate: "project_context",
		Subject:   frame.ProjectID,
		Object:    fmt.Sprintf("symbols_analyzed:%d", count),
		Weight:    0.5,
	})

	if frame.SymbolID != "" {
		frame.Context = append(frame.Context, Atom{
			Predicate: "focus_symbol",
			Subject:   frame.ID.String(),
			Object:    frame.SymbolID,
			Weight:    1.0,
		})
	}

	frame.RawContext["store"] = store

	return nil
}

// buildFileMap creates a map of symbol -> file path for quick lookup
// It scans both Global (Attention Sink) and Window (Sliding Window) TopicIDs unless stickyOnlyMode is enabled
func (o *GraphOrienter) buildFileMap(ctx context.Context, store *meb.MEBStore, projectID string) map[string]string {
	fileMap := make(map[string]string)

	globalID := manager.GlobalTopicID(projectID)
	windowID := manager.WindowTopicID(projectID)

	for item := range scanFactsWithTopicID(ctx, store, globalID, "", "defines", "") {
		if sym, ok := item.Fact.Object.(string); ok {
			fileMap[sym] = item.Fact.Subject
		}
	}

	// Only scan WindowTopicID if not in sticky-only mode
	if !o.stickyOnlyMode {
		for item := range scanFactsWithTopicID(ctx, store, windowID, "", "defines", "") {
			if sym, ok := item.Fact.Object.(string); ok {
				fileMap[sym] = item.Fact.Subject
			}
		}
	}

	return fileMap
}

// scanWithLimit scans for inbound references (who calls this symbol)
// It scans both Global and Window TopicIDs and merges results
func (o *GraphOrienter) scanWithLimit(ctx context.Context, store *meb.MEBStore, symbol string, predicate string, limit int, projectID string) map[string]bool {
	results := make(map[string]bool)

	globalID := manager.GlobalTopicID(projectID)
	windowID := manager.WindowTopicID(projectID)

	count := 0
	for item := range scanFactsWithTopicID(ctx, store, globalID, "", predicate, symbol) {
		if count >= limit {
			break
		}
		results[item.Fact.Subject] = true
		count++
	}

	count = 0
	for item := range scanFactsWithTopicID(ctx, store, windowID, "", predicate, symbol) {
		if count >= limit {
			break
		}
		results[item.Fact.Subject] = true
		count++
	}

	return results
}

// scanWithLimitOutgoing scans for outbound references (what this symbol calls)
// It scans both Global and Window TopicIDs and merges results
func (o *GraphOrienter) scanWithLimitOutgoing(ctx context.Context, store *meb.MEBStore, symbol string, limit int, projectID string) map[string]bool {
	results := make(map[string]bool)

	globalID := manager.GlobalTopicID(projectID)
	windowID := manager.WindowTopicID(projectID)

	count := 0
	for item := range scanFactsWithTopicID(ctx, store, globalID, symbol, "calls", "") {
		if count >= limit {
			break
		}
		if obj, ok := item.Fact.Object.(string); ok {
			results[obj] = true
		}
		count++
	}

	count = 0
	for item := range scanFactsWithTopicID(ctx, store, windowID, symbol, "calls", "") {
		if count >= limit {
			break
		}
		if obj, ok := item.Fact.Object.(string); ok {
			results[obj] = true
		}
		count++
	}

	return results
}

// centralityResult holds centrality data for a symbol
type centralityResult struct {
	symbol    string
	inDegree  int
	outDegree int
	score     float64
}

// computeDegreeCentrality computes centrality scores for a set of symbols
// Higher scores = more architecturally significant
// It scans both Global and Window TopicIDs for complete coverage unless stickyOnlyMode is enabled
func (o *GraphOrienter) computeDegreeCentrality(ctx context.Context, store *meb.MEBStore, symbols []string, projectID string) map[string]float64 {
	inDegree := make(map[string]int)
	outDegree := make(map[string]int)

	globalID := manager.GlobalTopicID(projectID)
	windowID := manager.WindowTopicID(projectID)

	// Scan calls from both TopicIDs (or Global only if stickyOnlyMode)
	for item := range scanFactsWithTopicID(ctx, store, globalID, "", config.PredicateCalls, "") {
		if obj, ok := item.Fact.Object.(string); ok {
			inDegree[obj]++
		}
		outDegree[item.Fact.Subject]++
	}

	if !o.stickyOnlyMode {
		for item := range scanFactsWithTopicID(ctx, store, windowID, "", config.PredicateCalls, "") {
			if obj, ok := item.Fact.Object.(string); ok {
				inDegree[obj]++
			}
			outDegree[item.Fact.Subject]++
		}
	}

	// Scan imports from both TopicIDs (or Global only if stickyOnlyMode)
	for item := range scanFactsWithTopicID(ctx, store, globalID, "", config.PredicateImports, "") {
		if obj, ok := item.Fact.Object.(string); ok {
			inDegree[obj]++
		}
		outDegree[item.Fact.Subject]++
	}

	if !o.stickyOnlyMode {
		for item := range scanFactsWithTopicID(ctx, store, windowID, "", config.PredicateImports, "") {
			if obj, ok := item.Fact.Object.(string); ok {
				inDegree[obj]++
			}
			outDegree[item.Fact.Subject]++
		}
	}

	symbolSet := make(map[string]bool)
	for _, s := range symbols {
		symbolSet[s] = true
	}

	scores := make(map[string]float64)
	maxScore := 0.0

	for _, sym := range symbols {
		in := float64(inDegree[sym])
		out := float64(outDegree[sym])

		boost := 1.0
		lower := strings.ToLower(sym)
		if strings.Contains(lower, ":main") || strings.Contains(lower, ".main") ||
			strings.Contains(lower, ":init") || strings.Contains(lower, ".init") {
			boost *= 2.5
		}

		if out > 10 && in > 5 {
			boost *= 1.5
		}

		if isInterfacePattern(sym) {
			boost *= 1.3
		}

		// Secondary attention filter: boost attention-worthy names
		if isAttentionWorthyName(sym) {
			boost *= 1.2
		}

		score := (in + out) * boost
		scores[sym] = score

		if score > maxScore {
			maxScore = score
		}
	}

	if maxScore > 0 {
		for sym := range scores {
			scores[sym] = scores[sym] / maxScore
		}
	}

	return scores
}

// isInterfacePattern checks if a symbol name matches interface-like patterns
func isInterfacePattern(symbol string) bool {
	lower := strings.ToLower(symbol)
	patterns := []string{
		"interface", "handler", "service", "repository",
		"controller", "provider", "client", "adapter",
		"factory", "strategy", "observer", "listener",
		"plugin", "middleware", "builder", "parser", "validator",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// isAttentionWorthyName returns true if the symbol name matches attention-worthy patterns.
// Used as a secondary filter in the virtual attention sink to catch symbols that
// centrality scoring might miss (e.g., security-critical functions by name).
func isAttentionWorthyName(name string) bool {
	lower := strings.ToLower(name)

	// Entry point patterns
	if strings.Contains(lower, "main") || strings.Contains(lower, "init") || strings.Contains(lower, "start") {
		return true
	}

	// Event/Callback patterns
	if strings.Contains(lower, "handler") || strings.HasPrefix(lower, "on_") || strings.HasSuffix(lower, "handler") || strings.HasSuffix(lower, "callback") {
		return true
	}

	// Lifecycle patterns
	if strings.Contains(lower, "setup") || strings.Contains(lower, "teardown") || strings.Contains(lower, "bootstrap") || strings.Contains(lower, "cleanup") {
		return true
	}

	// Test patterns
	if strings.HasSuffix(name, "_test") || strings.HasSuffix(name, "Test") || strings.HasPrefix(name, "Test") || strings.HasSuffix(name, "_bench") || strings.HasSuffix(name, "Benchmark") {
		return true
	}

	// Security/Auth patterns
	if strings.Contains(lower, "auth") || strings.Contains(lower, "login") || strings.Contains(lower, "logout") || strings.Contains(lower, "password") || strings.Contains(lower, "credential") || strings.Contains(lower, "token") || strings.Contains(lower, "session") || strings.Contains(lower, "sanitize") || strings.Contains(lower, "authorize") {
		return true
	}

	// Validation patterns
	if strings.Contains(lower, "validate") || strings.Contains(lower, "verify") || strings.Contains(lower, "check") {
		return true
	}

	// CRUD patterns (prefix-based)
	if strings.HasPrefix(name, "Create") || strings.HasPrefix(name, "Delete") || strings.HasPrefix(name, "Update") || strings.HasPrefix(name, "Get") || strings.HasPrefix(name, "List") || strings.HasPrefix(name, "Put") {
		return true
	}

	// Error/Recovery patterns
	if strings.Contains(lower, "error") || strings.Contains(lower, "retry") || strings.Contains(lower, "fallback") || strings.Contains(lower, "recovery") || strings.Contains(lower, "recover") {
		return true
	}

	return false
}

// sortByCentralityDesc sorts symbols by centrality score in descending order
func sortByCentralityDesc(symbols []string, centrality map[string]float64) {
	for i := 0; i < len(symbols)-1; i++ {
		for j := i + 1; j < len(symbols); j++ {
			if centrality[symbols[i]] < centrality[symbols[j]] {
				symbols[i], symbols[j] = symbols[j], symbols[i]
			}
		}
	}
}
