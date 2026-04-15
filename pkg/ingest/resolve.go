package ingest

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/meb"
)

type SymbolResolver struct {
	store     *meb.MEBStore
	importMap map[string]string
}

func NewSymbolResolver(store *meb.MEBStore) *SymbolResolver {
	return &SymbolResolver{
		store:     store,
		importMap: make(map[string]string),
	}
}

func (sr *SymbolResolver) BuildSymbolIndex(store *meb.MEBStore) error {
	return nil
}

func (sr *SymbolResolver) BuildImportMap(store *meb.MEBStore) error {
	ctx := context.Background()
	for fact, err := range store.ScanContext(ctx, "", config.PredicateImports, "") {
		if err != nil {
			continue
		}
		fileID := fact.Subject
		importPath, ok := fact.Object.(string)
		if !ok {
			continue
		}
		sr.importMap[fileID+"::"+importPath] = importPath
		sr.importMap[importPath] = importPath
	}
	return nil
}

func (sr *SymbolResolver) ResolveCallee(callerFile, calleeName string) string {
	if calleeName == "" {
		return ""
	}

	calleeName = strings.TrimSpace(calleeName)
	ctx := context.Background()

	if sym, ok := sr.findSymbolByFullID(ctx, calleeName); ok {
		return sym
	}

	parts := strings.Split(calleeName, ".")
	if len(parts) >= 2 {
		pkg := parts[0]
		member := parts[len(parts)-1]

		candidates := sr.findSymbolsByShortName(ctx, member)
		for _, sym := range candidates {
			if strings.HasSuffix(sym, ":"+member) || strings.HasSuffix(sym, "."+member) {
				if strings.Contains(sym, pkg) || fileContainsPkg(sr.getSymbolFile(ctx, sym), pkg) {
					return sym
				}
			}
		}

		if sym := sr.findBestCandidate(ctx, candidates, pkg, member, callerFile); sym != "" {
			return sym
		}
	}

	baseName := filepath.Base(calleeName)
	candidates := sr.findSymbolsByShortName(ctx, baseName)
	if sym := sr.findBestCandidate(ctx, candidates, "", baseName, callerFile); sym != "" {
		return sym
	}

	return calleeName
}

func (sr *SymbolResolver) findSymbolByFullID(ctx context.Context, fullID string) (string, bool) {
	for fact, err := range sr.store.ScanContext(ctx, fullID, config.PredicateDefines, "") {
		if err != nil {
			continue
		}
		if fact.Subject == fullID {
			return fact.Subject, true
		}
	}
	return "", false
}

func (sr *SymbolResolver) findSymbolsByShortName(ctx context.Context, shortName string) []string {
	var results []string
	for subject := range sr.store.FindSubjectsByObject(ctx, config.PredicateHasName, shortName) {
		results = append(results, subject)
	}
	return results
}

func (sr *SymbolResolver) getSymbolFile(ctx context.Context, symbolID string) string {
	for fact, err := range sr.store.ScanContext(ctx, "", config.PredicateDefines, symbolID) {
		if err != nil {
			continue
		}
		if obj, ok := fact.Object.(string); ok && obj == symbolID {
			return fact.Subject
		}
	}
	return ""
}

func (sr *SymbolResolver) findBestCandidate(ctx context.Context, candidates []string, pkg, shortName, callerFile string) string {
	if len(candidates) == 0 {
		return ""
	}
	if len(candidates) == 1 {
		return candidates[0]
	}

	callerDir := filepath.Dir(callerFile)
	bestScore := -1
	bestCandidate := ""

	for _, sym := range candidates {
		score := sr.scoreCandidate(sym, pkg, shortName, callerDir)
		if score > bestScore {
			bestScore = score
			bestCandidate = sym
		}
	}

	return bestCandidate
}

func (sr *SymbolResolver) scoreCandidate(sym, pkg, shortName, callerDir string) int {
	score := 0

	symDir := filepath.Dir(sym)
	if symDir == callerDir {
		score += 100
	}

	parentDir := filepath.Dir(symDir)
	if strings.Contains(parentDir, pkg) {
		score += 50
	}

	if strings.HasSuffix(sym, ":"+shortName) || strings.HasSuffix(sym, "."+shortName) {
		score += 25
	}

	return score
}

func fileContainsPkg(file, pkg string) bool {
	dir := filepath.Dir(file)
	dir = strings.ReplaceAll(dir, string(filepath.Separator), "/")
	pkgParts := strings.Split(pkg, "/")
	for i := len(pkgParts) - 1; i >= 0; i-- {
		if strings.Contains(dir, pkgParts[i]) {
			return true
		}
	}
	return false
}

func extractSymbolName(symID string) string {
	parts := strings.Split(symID, ":")
	if len(parts) >= 2 {
		return parts[1]
	}
	return symID
}

func extractShortName(symID string) string {
	parts := strings.Split(symID, ":")
	if len(parts) >= 2 {
		name := parts[1]
		if idx := strings.LastIndex(name, "."); idx != -1 {
			return name[idx+1:]
		}
		return name
	}
	return symID
}

type ResolvedCall struct {
	Caller   string
	Callee   string
	Resolved bool
	Line     int
}

func (sr *SymbolResolver) ResolveCalls(refs []Reference) []ResolvedCall {
	var resolved []ResolvedCall
	for _, ref := range refs {
		if ref.Predicate != config.PredicateCalls {
			continue
		}
		caller := ref.Subject
		if caller == "" {
			continue
		}
		callee := ref.Object
		if callee == "" {
			continue
		}

		resolvedCallee := sr.ResolveCallee(caller, callee)
		resolved = append(resolved, ResolvedCall{
			Caller:   caller,
			Callee:   callee,
			Resolved: resolvedCallee != callee,
			Line:     ref.Line,
		})

		ref.Object = resolvedCallee
	}
	return resolved
}

type CallGraph struct {
	Calls    map[string][]string
	CalledBy map[string][]string
}

func (sr *SymbolResolver) BuildCallGraph(store *meb.MEBStore) (*CallGraph, error) {
	cg := &CallGraph{
		Calls:    make(map[string][]string),
		CalledBy: make(map[string][]string),
	}

	if err := sr.BuildSymbolIndex(store); err != nil {
		return nil, err
	}
	sr.BuildImportMap(store)

	ctx := context.Background()
	for fact, err := range store.ScanContext(ctx, "", config.PredicateCalls, "") {
		if err != nil {
			continue
		}
		caller := fact.Subject
		calleeRaw, ok := fact.Object.(string)
		if !ok {
			continue
		}

		callee := sr.ResolveCallee(caller, calleeRaw)
		if callee == "" {
			continue
		}

		cg.Calls[caller] = append(cg.Calls[caller], callee)
		cg.CalledBy[callee] = append(cg.CalledBy[callee], caller)
	}

	return cg, nil
}

func (cg *CallGraph) GetCallees(symbol string) []string {
	if calls, ok := cg.Calls[symbol]; ok {
		return calls
	}
	return nil
}

func (cg *CallGraph) GetCallers(symbol string) []string {
	if callers, ok := cg.CalledBy[symbol]; ok {
		return callers
	}
	return nil
}

func (cg *CallGraph) GetCallersRecursive(symbol string, maxDepth int) []string {
	visited := make(map[string]bool)
	var result []string
	cg.collectCallers(symbol, 0, maxDepth, visited, &result)
	return result
}

func (cg *CallGraph) collectCallers(symbol string, depth, maxDepth int, visited map[string]bool, result *[]string) {
	if depth >= maxDepth {
		return
	}
	callers := cg.GetCallers(symbol)
	for _, caller := range callers {
		if !visited[caller] {
			visited[caller] = true
			*result = append(*result, caller)
			cg.collectCallers(caller, depth+1, maxDepth, visited, result)
		}
	}
}

func (cg *CallGraph) GetCalleesRecursive(symbol string, maxDepth int) []string {
	visited := make(map[string]bool)
	var result []string
	cg.collectCallees(symbol, 0, maxDepth, visited, &result)
	return result
}

func (cg *CallGraph) collectCallees(symbol string, depth, maxDepth int, visited map[string]bool, result *[]string) {
	if depth >= maxDepth {
		return
	}
	callees := cg.GetCallees(symbol)
	for _, callee := range callees {
		if !visited[callee] {
			visited[callee] = true
			*result = append(*result, callee)
			cg.collectCallees(callee, depth+1, maxDepth, visited, result)
		}
	}
}

func (cg *CallGraph) DetectCycles() [][]string {
	var cycles [][]string
	visited := make(map[string]bool)
	inStack := make(map[string]bool)
	path := []string{}

	var dfs func(node string) bool
	dfs = func(node string) bool {
		if inStack[node] {
			cycleStart := -1
			for i, n := range path {
				if n == node {
					cycleStart = i
					break
				}
			}
			if cycleStart >= 0 {
				cycle := make([]string, len(path)-cycleStart)
				copy(cycle, path[cycleStart:])
				cycle = append(cycle, node)
				cycles = append(cycles, cycle)
			}
			return true
		}
		if visited[node] {
			return false
		}

		visited[node] = true
		inStack[node] = true
		path = append(path, node)

		for _, callee := range cg.GetCallees(node) {
			dfs(callee)
		}

		path = path[:len(path)-1]
		inStack[node] = false
		return false
	}

	for node := range cg.Calls {
		if !visited[node] {
			dfs(node)
		}
	}

	return cycles
}

func (cg *CallGraph) FindReachable(from, to string, maxDepth int) bool {
	visited := make(map[string]bool)
	queue := []string{from}
	visited[from] = true
	depth := make(map[string]int)
	depth[from] = 0

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current == to {
			return true
		}

		if depth[current] >= maxDepth {
			continue
		}

		for _, callee := range cg.GetCallees(current) {
			if !visited[callee] {
				visited[callee] = true
				depth[callee] = depth[current] + 1
				queue = append(queue, callee)
			}
		}
	}
	return false
}

func (cg *CallGraph) LeastCommonAncestor(a, b string, maxDepth int) string {
	aAncestors := make(map[string]int)
	queue := []string{a}
	aAncestors[a] = 0
	depth := 0

	for len(queue) > 0 && depth < maxDepth {
		size := len(queue)
		for i := 0; i < size; i++ {
			current := queue[0]
			queue = queue[1:]
			for _, caller := range cg.GetCallers(current) {
				if _, exists := aAncestors[caller]; !exists {
					aAncestors[caller] = depth + 1
					queue = append(queue, caller)
				}
			}
		}
		depth++
	}

	bVisited := make(map[string]bool)
	queue = []string{b}
	bVisited[b] = true
	depth = 0

	for len(queue) > 0 && depth < maxDepth {
		size := len(queue)
		for i := 0; i < size; i++ {
			current := queue[0]
			queue = queue[1:]
			if depthA, exists := aAncestors[current]; exists && depthA == depth {
				return current
			}
			for _, caller := range cg.GetCallers(current) {
				if !bVisited[caller] {
					bVisited[caller] = true
					queue = append(queue, caller)
				}
			}
		}
		depth++
	}

	return ""
}

func AddResolvedCallsAsCalledBy(store *meb.MEBStore, cg *CallGraph) error {
	for callee, callers := range cg.CalledBy {
		for _, caller := range callers {
			fact := meb.Fact{
				Subject:   callee,
				Predicate: config.PredicateCalledBy,
				Object:    caller,
			}
			if err := store.AddFact(fact); err != nil {
				fmt.Printf("[Resolve] Warning: failed to add called_by fact for %s <- %s: %v\n", callee, caller, err)
			}
		}
	}
	return nil
}
