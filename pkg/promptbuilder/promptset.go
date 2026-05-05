package promptbuilder

import (
	"github.com/duynguyendang/gca/pkg/prompts"
)

type PromptSet struct {
	Datalog        *prompts.Prompt
	Chat           *prompts.Prompt
	PathNarrative  *prompts.Prompt
	PathEndpoints  *prompts.Prompt
	ResolveSymbol  *prompts.Prompt
	Prune          *prompts.Prompt
	SmartSearch    *prompts.Prompt
	MultiFile      *prompts.Prompt
	DefaultContext *prompts.Prompt
	Insight        *prompts.Prompt
	Summary        *prompts.Prompt
	Narrative      *prompts.Prompt
	Refactor       *prompts.Prompt
	TestGen        *prompts.Prompt
	Security       *prompts.Prompt
	Performance    *prompts.Prompt
}

func (ps *PromptSet) AllTasks() []string {
	return []string{
		"insight", "chat", "prune", "summary", "narrative",
		"resolve_symbol", "path_endpoints", "datalog", "path_narrative",
		"smart_search_analysis", "multi_file_summary", "refactor",
		"test_generation", "security_audit", "performance",
	}
}