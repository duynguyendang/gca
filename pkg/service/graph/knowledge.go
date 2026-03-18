package graph

import (
	"context"
	"fmt"

	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/vector"
)

type KnowledgeGraph struct {
	store *meb.MEBStore
}

func NewKnowledgeGraph(store *meb.MEBStore) *KnowledgeGraph {
	return &KnowledgeGraph{
		store: store,
	}
}

func (kg *KnowledgeGraph) AddFact(subject, predicate string, object any, graph string) error {
	fact := meb.NewFactInGraph(subject, predicate, object, graph)
	return kg.store.AddFact(fact)
}

func (kg *KnowledgeGraph) AddFacts(facts []meb.Fact) error {
	return kg.store.AddFactBatch(facts)
}

func (kg *KnowledgeGraph) Query(ctx context.Context, datalogQuery string) ([]map[string]any, error) {
	return kg.store.Query(ctx, datalogQuery)
}

func (kg *KnowledgeGraph) QueryDatalog(ctx context.Context, datalogQuery string) ([]map[string]string, error) {
	return kg.store.QueryDatalog(ctx, datalogQuery)
}

type ScanOptions struct {
	Subject   string
	Predicate string
	Object    string
	Graph     string
}

func (kg *KnowledgeGraph) Scan(opts ScanOptions) ([]meb.Fact, error) {
	var results []meb.Fact

	iter := kg.store.Scan(
		opts.Subject,
		opts.Predicate,
		opts.Object,
		opts.Graph,
	)

	for fact, err := range iter {
		if err != nil {
			return results, err
		}
		results = append(results, fact)
	}

	return results, nil
}

type SemanticSearchOptions struct {
	Embedding []float32
	Threshold float32
	Limit     int
}

type SearchResult struct {
	ID      uint64
	Score   float32
	Subject string
}

func (kg *KnowledgeGraph) SemanticSearch(ctx context.Context, opts SemanticSearchOptions) ([]SearchResult, error) {
	embedding := opts.Embedding

	if len(embedding) == 0 {
		return nil, fmt.Errorf("embedding required for semantic search")
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 10
	}

	vectorResults, err := kg.store.Vectors().Search(embedding, limit*10)
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}

	results := make([]SearchResult, 0, len(vectorResults))
	for _, vr := range vectorResults {
		if opts.Threshold > 0 && vr.Score < opts.Threshold {
			continue
		}

		subject, err := kg.store.ResolveID(vr.ID)
		if err != nil {
			continue
		}

		results = append(results, SearchResult{
			ID:      vr.ID,
			Score:   vr.Score,
			Subject: subject,
		})

		if len(results) >= limit {
			break
		}
	}

	return results, nil
}

type CommunityResult struct {
	Level          int
	NumCommunities int
}

func (kg *KnowledgeGraph) DetectCommunities(graphID string) ([]CommunityResult, error) {
	if graphID == "" {
		graphID = "default"
	}

	hierarchy, err := kg.store.DetectCommunities(graphID)
	if err != nil {
		return nil, err
	}

	results := make([]CommunityResult, len(hierarchy.Levels))
	for i, level := range hierarchy.Levels {
		results[i] = CommunityResult{
			Level:          i,
			NumCommunities: len(level),
		}
	}

	return results, nil
}

func (kg *KnowledgeGraph) GetCommunityMembers(graphID string, level uint8, commID uint64) ([]uint64, error) {
	if graphID == "" {
		graphID = "default"
	}
	return kg.store.GetCommunityMembers(graphID, level, commID)
}

func (kg *KnowledgeGraph) GetNodeCommunityPath(graphID string, nodeID uint64) ([]uint64, error) {
	if graphID == "" {
		graphID = "default"
	}
	return kg.store.GetNodeCommunityPath(graphID, nodeID)
}

type HybridClusterResult struct {
	Clusters []ClusterInfo
}

type ClusterInfo struct {
	ID      int
	Members []string
}

func (kg *KnowledgeGraph) HybridCluster(ctx context.Context, embedding []float32, limit, numClusters int) (*HybridClusterResult, error) {
	if limit <= 0 {
		limit = 50
	}
	if numClusters <= 0 {
		numClusters = 5
	}

	result, err := kg.store.ClusterWithHybrid("default", embedding, limit, numClusters)
	if err != nil {
		return nil, err
	}

	clusters := make([]ClusterInfo, len(result.Clusters))
	for i, c := range result.Clusters {
		members := make([]string, len(c.Members))
		for j, m := range c.Members {
			members[j] = fmt.Sprintf("%d", m)
		}
		clusters[i] = ClusterInfo{
			ID:      i,
			Members: members,
		}
	}

	return &HybridClusterResult{
		Clusters: clusters,
	}, nil
}

func (kg *KnowledgeGraph) GetContent(id string) ([]byte, error) {
	dictID, found := kg.store.LookupID(id)
	if !found {
		return nil, fmt.Errorf("id not found: %s", id)
	}
	return kg.store.GetContent(dictID)
}

func (kg *KnowledgeGraph) LookupID(id string) (uint64, bool) {
	return kg.store.LookupID(id)
}

func (kg *KnowledgeGraph) ResolveID(id uint64) (string, error) {
	return kg.store.ResolveID(id)
}

func (kg *KnowledgeGraph) Vectors() *vector.VectorRegistry {
	return kg.store.Vectors()
}

func (kg *KnowledgeGraph) Store() *meb.MEBStore {
	return kg.store
}

func (kg *KnowledgeGraph) GetFactsForSubject(subject string) ([]meb.Fact, error) {
	return kg.Scan(ScanOptions{
		Subject: subject,
		Graph:   "default",
	})
}

func (kg *KnowledgeGraph) GetFactsByPredicate(predicate string) ([]meb.Fact, error) {
	return kg.Scan(ScanOptions{
		Predicate: predicate,
		Graph:     "default",
	})
}

func (kg *KnowledgeGraph) GetIncomingEdges(subject string) ([]meb.Fact, error) {
	return kg.Scan(ScanOptions{
		Object: subject,
		Graph:  "default",
	})
}

func (kg *KnowledgeGraph) GetOutgoingEdges(subject string) ([]meb.Fact, error) {
	return kg.Scan(ScanOptions{
		Subject: subject,
		Graph:   "default",
	})
}
