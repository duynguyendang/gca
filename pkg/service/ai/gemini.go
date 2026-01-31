package ai

import (
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/duynguyendang/gca/pkg/meb"
	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// ProjectStoreManager interface abstraction to avoid circular dependency if possible,
// or just use the one from service package if it's exported.
// Since we are in `pkg/service/ai`, we can't import `pkg/service`.
// We will define a local interface or rely on `meb.MEBStore`.
type ProjectStoreManager interface {
	GetStore(projectID string) (*meb.MEBStore, error)
}

type GeminiService struct {
	client  *genai.Client
	model   *genai.GenerativeModel
	manager ProjectStoreManager
}

func NewGeminiService(ctx context.Context, apiKey string, manager ProjectStoreManager) (*GeminiService, error) {
	if apiKey == "" {
		apiKey = os.Getenv("GEMINI_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY not found")
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create gemini client: %w", err)
	}

	// Use a default model, can be configured later
	modelName := os.Getenv("GEMINI_MODEL")
	if modelName == "" {
		modelName = "gemini-2.0-flash-exp"
	}
	model := client.GenerativeModel(modelName)
	model.SetTemperature(0.2) // Low temperature for technical accuracy

	return &GeminiService{
		client:  client,
		model:   model,
		manager: manager,
	}, nil
}

// Ask processes a user query, optionally focusing on a specific symbol.
// AIRequest defines the structure for AI operations
type AIRequest struct {
	ProjectID string      `json:"project_id"`
	Task      string      `json:"task"` // "chat", "insight", "prune", "summary", "path", etc.
	Query     string      `json:"query"`
	SymbolID  string      `json:"symbol_id"`
	Data      interface{} `json:"data"` // Flexible data payload (node list, etc.)
}

// HandleRequest dispatchs the AI request based on the Task type
func (s *GeminiService) HandleRequest(ctx context.Context, req AIRequest) (string, error) {
	store, err := s.manager.GetStore(req.ProjectID)
	if err != nil {
		return "", fmt.Errorf("failed to get store: %w", err)
	}

	prompt, err := s.buildTaskPrompt(ctx, store, req)
	if err != nil {
		return "", fmt.Errorf("failed to build prompt: %w", err)
	}

	resp, err := s.model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		log.Printf("Gemini GenerateContent Failed: %v", err)
		return "", fmt.Errorf("gemini request failed: %w", err)
	}

	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		log.Printf("Gemini returned empty candidates")
		return "No response from AI.", nil
	}

	var sb strings.Builder
	for _, part := range resp.Candidates[0].Content.Parts {
		if txt, ok := part.(genai.Text); ok {
			sb.WriteString(string(txt))
		}
	}

	return sb.String(), nil
}

func (s *GeminiService) buildTaskPrompt(ctx context.Context, store *meb.MEBStore, req AIRequest) (string, error) {
	switch req.Task {
	case "insight": // Node Insight
		// Query: "Analyze the architectural role of component..."
		// We can construct this standard prompt here.
		return s.BuildPrompt(ctx, store, fmt.Sprintf("Analyze the architectural role of component %s. Provide a comprehensive analysis including role, interactions, and design patterns.", req.SymbolID), req.SymbolID)

	case "prune": // Prune Nodes
		nodeList, ok := req.Data.(string) // Expecting a string formatted list or we format it here?
		// Better to accept formatted string for now to match FE logic or []interface{}
		if !ok {
			// If it's a list of objects, format it
			if list, ok := req.Data.([]interface{}); ok {
				var sb strings.Builder
				for _, item := range list {
					if m, ok := item.(map[string]interface{}); ok {
						name, _ := m["name"].(string)
						kind, _ := m["kind"].(string)
						id, _ := m["id"].(string)
						sb.WriteString(fmt.Sprintf("- %s (Kind: %s, ID: %s)\n", name, kind, id))
					}
				}
				nodeList = sb.String()
			}
		}

		return fmt.Sprintf(`Identify the TOP 7 most significant "architectural gateway" nodes from this list:
%s

Return strictly JSON format:
{
    "selectedIds": ["id1", "id2"],
    "explanation": "Brief reason..."
}`, nodeList), nil

	case "summary": // Architecture Summary for File
		// req.Query could be fileName
		nodeListStr := ""
		if list, ok := req.Data.([]interface{}); ok {
			var sb strings.Builder
			for i, item := range list {
				if i >= 15 {
					break
				}
				if m, ok := item.(map[string]interface{}); ok {
					name, _ := m["name"].(string)
					kind, _ := m["kind"].(string)
					sb.WriteString(fmt.Sprintf("- %s (%s)\n", name, kind))
				}
			}
			nodeListStr = sb.String()
		}
		return s.BuildPrompt(ctx, store, fmt.Sprintf("Provide a 2-3 sentence architectural summary for file \"%s\".\nSymbols:\n%s", req.Query, nodeListStr), "")

	case "narrative": // Architecture Flow Narrative
		// Data is nodes list
		nodeNames := ""
		if list, ok := req.Data.([]interface{}); ok {
			names := make([]string, 0, len(list))
			for _, item := range list {
				if m, ok := item.(map[string]interface{}); ok {
					if name, ok := m["name"].(string); ok {
						names = append(names, name)
					}
				}
			}
			nodeNames = strings.Join(names, ", ")
		}
		return s.BuildPrompt(ctx, store, fmt.Sprintf("Explain the high-level logic flow for these components: %s. Keep it concise.", nodeNames), "")

	case "resolve_symbol":
		candidateStr := ""
		if list, ok := req.Data.([]interface{}); ok {
			formatted := make([]string, 0)
			for i, item := range list {
				if i >= 30 {
					break
				}
				if s, ok := item.(string); ok {
					formatted = append(formatted, s)
				}
			}
			candidateStr = strings.Join(formatted, "\n")
		}
		return fmt.Sprintf(`Select the best matching symbol for query: "%s"
Candidates:
%s

Return ONLY the exact symbol ID. If no match, return "null".`, req.Query, candidateStr), nil

	case "path_endpoints":
		candidateStr := ""
		if list, ok := req.Data.([]interface{}); ok {
			formatted := make([]string, 0)
			for i, item := range list {
				if i >= 50 {
					break
				}
				if s, ok := item.(string); ok {
					formatted = append(formatted, s)
				}
			}
			candidateStr = strings.Join(formatted, "\n")
		}
		return fmt.Sprintf(`Identify Source and Target symbols for query: "%s"
Candidates:
%s

Return strictly JSON: { "from": "id", "to": "id" } or null.`, req.Query, candidateStr), nil

	case "datalog":
		return fmt.Sprintf(`You are a Datalog Query Generator for a Code Knowledge Graph.
Schema:
- Fact: triples(Subject, Predicate, Object)
- Predicates: "defines", "calls", "imports", "type", "implements", "has_doc", "in_package", "has_tag", "has_role", "calls_api", "handled_by", "exports", "exposes_model", "references".
- Meaning:
  - defines: File -> Symbol
  - calls: Execution flow (Func -> Func)
  - imports: File dependency
  - type: Node classification (e.g., "struct", "function")
  - in_package: Logical grouping
  - has_role: "api_handler", "data_contract", "entry_point", "utility"
  - has_tag: "backend", "frontend"
  - calls_api: Frontend -> URI
  - handled_by: URI -> Backend Handler
- Subject/Object are string identifiers (e.g., "pkg/file.go:Function").

Rules:
1. Use standard Datalog syntax.
2. Direct calls: triples(?caller, "calls", ?callee).
3. Transitive reachability (if asked):
   reachable(X, Y) :- triples(X, "calls", Y).
   reachable(X, Z) :- triples(X, "calls", Y), reachable(Y, Z).
4. Return ONLY the Datalog query inside a markdown block.

User Request: "%s"
Context Symbol: %s`, req.Query, req.SymbolID), nil

	case "path_narrative":
		// Query is flow description, Data is pathNodes
		pathStr := ""
		if list, ok := req.Data.([]interface{}); ok {
			names := make([]string, 0)
			for _, item := range list {
				if m, ok := item.(map[string]interface{}); ok {
					if name, ok := m["name"].(string); ok {
						names = append(names, name)
					}
				}
			}
			pathStr = strings.Join(names, " -> ")
		}
		return s.BuildPrompt(ctx, store, fmt.Sprintf("Explain flow: %s. Path: %s", req.Query, pathStr), "")

	default: // "chat" or unknown
		return s.BuildPrompt(ctx, store, req.Query, req.SymbolID)
	}
}

// BuildPrompt constructs the prompt with context.
// Requirement: < 50ms
func (s *GeminiService) BuildPrompt(ctx context.Context, store *meb.MEBStore, query string, symbolID string) (string, error) {
	startTime := time.Now()
	defer func() {
		fmt.Printf("BuildPrompt took %v\n", time.Since(startTime))
	}()

	var contextBuilder strings.Builder
	contextBuilder.WriteString("## Context\n")

	// 1. Direct Symbol Context (if provided)
	if symbolID != "" {
		if err := s.appendSymbolContext(ctx, store, symbolID, &contextBuilder); err != nil {
			log.Printf("Failed to fetch symbol context for %s: %v", symbolID, err)
		}
	} else {
		// 2. Semantic Context Discovery (from query)
		// Find potential symbols in query and fetch their 1-hop context
		// Optimization: Use LookupID (Exact Match) instead of SearchSymbols (Full Scan)

		words := extractPotentialSymbols(query)
		if len(words) > 0 {
			// Limit to top 3 unique matches
			count := 0
			seen := make(map[string]bool)

			var wg sync.WaitGroup
			var mu sync.Mutex

			for _, word := range words {
				if count >= 3 {
					break
				}
				if seen[word] {
					continue
				}
				seen[word] = true

				// Fast Is-It-A-Symbol Check
				// We check for exact match first.
				// This avoids scanning millions of keys.
				_, exists := store.LookupID(word)

				// Try variations if exact match fails?
				// e.g. "service" -> "pkg/service"?
				// For now, keep it simple and fast.

				if exists {
					// Convert numeric ID to string ID?
					// Wait, LookupID returns uint64. appendSymbolContext takes string ID.
					// We need ResolveID to get text back?
					// No, 'word' IS the string ID if it exists in dictionary (forward/reverse).
					// Actually LookupID verifies 'word' is in dictionary.
					// So we can pass 'word' as symbolID.

					matchedID := word
					if matchedID == symbolID {
						continue
					}

					wg.Add(1)
					go func(id string) {
						defer wg.Done()
						var localSb strings.Builder
						// Use a short timeout for each context fetch
						localCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
						defer cancel()

						if err := s.appendSymbolContext(localCtx, store, id, &localSb); err == nil {
							mu.Lock()
							contextBuilder.WriteString(localSb.String())
							mu.Unlock()
						}
					}(matchedID)
					count++
				}
			}
			wg.Wait()
		}

	}

	prompt := fmt.Sprintf(`You are an expert Software Architect assistant.
Assign context to the user's question using the provided Code and Graph information.

%s

## User Question
%s

Answer concisely and accurately based on the code provided.`, contextBuilder.String(), query)

	return prompt, nil
}

func (s *GeminiService) appendSymbolContext(ctx context.Context, store *meb.MEBStore, symbolID string, sb *strings.Builder) error {
	// 1. Fetch Symbol Content
	val, err := store.GetDocument(meb.DocumentID(symbolID))
	if err != nil {
		return err
	}
	content := string(val.Content)

	// 2. Run 1-hop Datalog queries
	// Inbound: Who calls me?
	inbound, _ := store.Query(ctx, fmt.Sprintf(`triples(?s, "calls", "%s")`, symbolID))

	// Outbound: Who do I call?
	outbound, _ := store.Query(ctx, fmt.Sprintf(`triples("%s", "calls", ?o)`, symbolID))

	// Defines: What do I define? (For files)
	defines, _ := store.Query(ctx, fmt.Sprintf(`triples("%s", "defines", ?o)`, symbolID))

	sb.WriteString(fmt.Sprintf("\n### Symbol: %s\n", symbolID))
	sb.WriteString("```go\n")
	// Truncate content if too long?
	if len(content) > 2000 {
		sb.WriteString(content[:2000] + "\n... (truncated)")
	} else {
		sb.WriteString(content)
	}
	sb.WriteString("\n```\n")

	if len(defines) > 0 {
		sb.WriteString("**Defines:**\n")
		for i, row := range defines {
			if i >= 5 {
				break
			}
			if obj, ok := row["?o"].(string); ok {
				sb.WriteString(fmt.Sprintf("- %s\n", obj))
			}
		}
	}

	if len(inbound) > 0 {
		sb.WriteString("**Called By:**\n")
		for i, row := range inbound {
			if i >= 5 {
				break
			}
			if subj, ok := row["?s"].(string); ok {
				sb.WriteString(fmt.Sprintf("- %s\n", subj))
			}
		}
	}

	if len(outbound) > 0 {
		sb.WriteString("**Calls:**\n")
		for i, row := range outbound {
			if i >= 5 {
				break
			}
			if obj, ok := row["?o"].(string); ok {
				sb.WriteString(fmt.Sprintf("- %s\n", obj))
			}
		}
	}
	sb.WriteString("\n")
	return nil
}

var symbolRegex = regexp.MustCompile(`\b[A-Za-z][A-Za-z0-9_.\/]{3,}\b`)

func extractPotentialSymbols(query string) []string {
	// Simple extraction: words >= 4 chars to avoid "are", "the", "any"
	// Prefer CamelCase or underscores/dots
	return symbolRegex.FindAllString(query, -1)
}
