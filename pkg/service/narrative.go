package service

import "github.com/duynguyendang/gca/pkg/service/ai"

// NilNarrative returns the AI service as a NarrativeService, or nil when the AI
// service is not configured. Passing a typed-nil *ai.AIService into an
// interface field would create a non-nil interface wrapping a nil pointer and
// panic on call, so callers must convert to a plain nil here.
func NilNarrative(as *ai.AIService) NarrativeService {
	if as == nil {
		return nil
	}
	return as
}
