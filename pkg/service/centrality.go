package service

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/meb"
)

type CentralityService struct {
	cache  map[string]map[string]float64
	mu     sync.RWMutex
	ttl    time.Duration
	enable bool
}

type CentralityResult struct {
	SymbolID   string  `json:"symbol_id"`
	Centrality float64 `json:"centrality"`
	InDegree   int     `json:"in_degree"`
	OutDegree  int     `json:"out_degree"`
	Kind       string  `json:"kind,omitempty"`
	IsEntry    bool    `json:"is_entry"`
}

func NewCentralityService() *CentralityService {
	return &CentralityService{
		cache:  make(map[string]map[string]float64),
		ttl:    config.CentralityCacheTTL,
		enable: config.CentralityEnabled,
	}
}

func (s *CentralityService) ComputeDegreeCentrality(ctx context.Context, store *meb.MEBStore) (map[string]float64, error) {
	if !s.enable {
		return nil, fmt.Errorf("centrality computation is disabled")
	}

	inDegree := make(map[string]int)
	outDegree := make(map[string]int)
	kindMap := make(map[string]string)

	for fact := range store.ScanContext(ctx, "", config.PredicateCalls, "") {
		if obj, ok := fact.Object.(string); ok {
			inDegree[obj]++
		}
		outDegree[fact.Subject]++
	}

	for fact := range store.ScanContext(ctx, "", config.PredicateDefines, "") {
		if sym, ok := fact.Object.(string); ok {
			kindMap[sym] = s.inferKind(sym)
		}
	}

	for fact := range store.ScanContext(ctx, "", config.PredicateImports, "") {
		if obj, ok := fact.Object.(string); ok {
			inDegree[obj]++
		}
		outDegree[fact.Subject]++
	}

	centrality := make(map[string]float64)
	allSymbols := make(map[string]bool)
	for sym := range inDegree {
		allSymbols[sym] = true
	}
	for sym := range outDegree {
		allSymbols[sym] = true
	}

	maxScore := 0.0
	for sym := range allSymbols {
		in := float64(inDegree[sym])
		out := float64(outDegree[sym])
		boost := s.architecturalBoost(sym, kindMap[sym], inDegree[sym], outDegree[sym])
		score := (config.CentralityBoostIn*in + config.CentralityBoostOut*out) * boost
		centrality[sym] = score
		if score > maxScore {
			maxScore = score
		}
	}

	if maxScore > 0 {
		for sym := range centrality {
			centrality[sym] = centrality[sym] / maxScore
		}
	}

	return centrality, nil
}

func (s *CentralityService) inferKind(symbol string) string {
	lower := strings.ToLower(symbol)
	if strings.HasSuffix(lower, ".main") || strings.HasSuffix(lower, ":main") {
		return "entry"
	}
	if strings.HasSuffix(lower, ".init") || strings.HasSuffix(lower, ":init") {
		return "entry"
	}
	parts := strings.Split(symbol, ":")
	if len(parts) >= 2 {
		last := strings.ToLower(parts[len(parts)-1])
		if strings.HasPrefix(last, "test") || strings.HasPrefix(last, "benchmark") {
			return "test"
		}
	}
	return "symbol"
}

func (s *CentralityService) architecturalBoost(symbol, kind string, inDeg, outDeg int) float64 {
	boost := 1.0

	lower := strings.ToLower(symbol)
	isMain := strings.Contains(lower, ":main") || strings.Contains(lower, ".main")
	isInit := strings.Contains(lower, ":init") || strings.Contains(lower, ".init")

	if isMain || isInit {
		boost *= config.CentralityBoostMain
	}

	if kind == "entry" {
		boost *= config.CentralityBoostEntry
	}

	if outDeg > 10 && inDeg > 5 {
		boost *= config.CentralityBoostHub
	}

	if IsInterfacePattern(symbol) {
		boost *= config.CentralityBoostInterface
	}

	return boost
}

func (s *CentralityService) GetTopK(ctx context.Context, store *meb.MEBStore, symbols []string, k int) ([]string, error) {
	if len(symbols) == 0 {
		return nil, nil
	}

	centrality, err := s.ComputeDegreeCentrality(ctx, store)
	if err != nil {
		return nil, err
	}

	sorted := make([]string, 0, len(symbols))
	for _, sym := range symbols {
		if _, ok := centrality[sym]; ok {
			sorted = append(sorted, sym)
		}
	}

	sortByCentralityDesc(sorted, centrality)

	if k > len(sorted) {
		k = len(sorted)
	}
	return sorted[:k], nil
}

func (s *CentralityService) GetScores(ctx context.Context, store *meb.MEBStore, symbols []string) (map[string]float64, error) {
	if len(symbols) == 0 {
		return make(map[string]float64), nil
	}

	centrality, err := s.ComputeDegreeCentrality(ctx, store)
	if err != nil {
		return nil, err
	}

	result := make(map[string]float64)
	for _, sym := range symbols {
		if score, ok := centrality[sym]; ok {
			result[sym] = score
		} else {
			result[sym] = 0.0
		}
	}
	return result, nil
}

func (s *CentralityService) SortByCentrality(ctx context.Context, store *meb.MEBStore, symbols []string) ([]string, error) {
	if len(symbols) == 0 {
		return symbols, nil
	}

	scores, err := s.GetScores(ctx, store, symbols)
	if err != nil {
		return nil, err
	}

	sorted := make([]string, len(symbols))
	copy(sorted, symbols)
	sortByCentralityDesc(sorted, scores)
	return sorted, nil
}

func sortByCentralityDesc(symbols []string, centrality map[string]float64) {
	n := len(symbols)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if centrality[symbols[j]] < centrality[symbols[j+1]] {
				symbols[j], symbols[j+1] = symbols[j+1], symbols[j]
			}
		}
	}
}

func (s *CentralityService) InvalidateCache(projectID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cache, projectID)
}

func (s *CentralityService) IsEnabled() bool {
	return s.enable
}

type InterfacePattern struct {
	interfaceNames []string
	mu             sync.RWMutex
	compiled       map[string]*regexp.Regexp
}

var defaultInterfacePattern = &InterfacePattern{
	interfaceNames: []string{
		"interface",
		"handler",
		"service",
		"repository",
		"controller",
		"provider",
		"client",
		"adapter",
		"factory",
		"strategy",
		"observer",
		"listener",
		"plugin",
		"middleware",
		"builder",
		"parser",
		"validator",
	},
	compiled: make(map[string]*regexp.Regexp),
}

func (p *InterfacePattern) Match(symbol string) bool {
	lower := strings.ToLower(symbol)
	for _, name := range p.interfaceNames {
		if strings.Contains(lower, name) {
			return true
		}
	}
	return false
}

var interfacePatternRegex = regexp.MustCompile(`(interface|handler|service|repository|controller|provider|client|adapter|factory|strategy|observer|listener|plugin|middleware|builder|parser|validator)`)

func IsInterfacePattern(symbol string) bool {
	return interfacePatternRegex.MatchString(strings.ToLower(symbol))
}

func ComputePageRankCentrality(ctx context.Context, store *meb.MEBStore, iterations int, damping float64) (map[string]float64, error) {
	if iterations <= 0 {
		iterations = 10
	}
	if damping <= 0 {
		damping = 0.85
	}

	nodes := make(map[string]struct{})
	edges := make(map[string][]string)

	for fact := range store.ScanContext(ctx, "", config.PredicateCalls, "") {
		if obj, ok := fact.Object.(string); ok {
			nodes[fact.Subject] = struct{}{}
			nodes[obj] = struct{}{}
			edges[fact.Subject] = append(edges[fact.Subject], obj)
		}
	}

	nodeList := make([]string, 0, len(nodes))
	for node := range nodes {
		nodeList = append(nodeList, node)
	}
	N := len(nodeList)
	if N == 0 {
		return make(map[string]float64), nil
	}

	nodeIndex := make(map[string]int)
	for i, node := range nodeList {
		nodeIndex[node] = i
	}

	ranks := make([]float64, N)
	for i := range ranks {
		ranks[i] = 1.0 / float64(N)
	}

	outLinks := make([]int, N)
	for i, node := range nodeList {
		outLinks[i] = len(edges[node])
	}

	for iter := 0; iter < iterations; iter++ {
		newRanks := make([]float64, N)

		for i := range nodeList {
			var sum float64
			for j, node := range nodeList {
				if outLinks[j] > 0 {
					for _, target := range edges[node] {
						if targetIdx, ok := nodeIndex[target]; ok && targetIdx == i {
							sum += damping * ranks[j] / float64(outLinks[j])
						}
					}
				}
			}
			newRanks[i] = (1.0-damping)/float64(N) + sum
		}

		ranks = newRanks
	}

	centrality := make(map[string]float64)
	maxRank := 0.0
	for i, node := range nodeList {
		centrality[node] = ranks[i]
		if ranks[i] > maxRank {
			maxRank = ranks[i]
		}
	}

	if maxRank > 0 {
		for node := range centrality {
			centrality[node] = centrality[node] / maxRank
		}
	}

	return centrality, nil
}

func NormalizeCentrality(scores map[string]float64) map[string]float64 {
	if len(scores) == 0 {
		return scores
	}

	maxScore := 0.0
	minScore := math.MaxFloat64

	for _, score := range scores {
		if score > maxScore {
			maxScore = score
		}
		if score < minScore {
			minScore = score
		}
	}

	rangeScore := maxScore - minScore
	if rangeScore == 0 {
		rangeScore = 1
	}

	normalized := make(map[string]float64)
	for node, score := range scores {
		normalized[node] = (score - minScore) / rangeScore
	}

	return normalized
}
