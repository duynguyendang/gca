package ooda

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/duynguyendang/gca/pkg/logger"
	"github.com/duynguyendang/manglekit/sdk/ooda"
)

type Phase = ooda.Phase

const (
	PhaseObserve Phase = ooda.PhaseObserve
	PhaseOrient  Phase = ooda.PhaseOrient
	PhaseDecide  Phase = ooda.PhaseDecide
	PhaseVerify  Phase = ooda.PhaseVerify
	PhaseAct     Phase = ooda.PhaseAct
)

type TaskType = ooda.TaskType

const (
	TaskTypeInduction  TaskType = ooda.TaskTypeInduction
	TaskTypeGeneration TaskType = ooda.TaskTypeGeneration
	TaskTypeAudit      TaskType = ooda.TaskTypeAudit
	TaskTypeRecovery   TaskType = ooda.TaskTypeRecovery
)

type VerifyStatus = ooda.VerifyStatus

const (
	VerifyStatusPending VerifyStatus = ooda.VerifyStatusPending
	VerifyStatusPassed  VerifyStatus = ooda.VerifyStatusPassed
	VerifyStatusFailed  VerifyStatus = ooda.VerifyStatusFailed
	VerifyStatusWarning VerifyStatus = ooda.VerifyStatusWarning
)

type IntentStr = ooda.IntentStr

type Atom = ooda.Atom

type DomainGene = ooda.DomainGene

type AuditResult = ooda.AuditResult

type CognitiveFrame = ooda.CognitiveFrame

func NewCognitiveFrame(input string, intent IntentStr, taskType TaskType) *CognitiveFrame {
	return ooda.NewCognitiveFrame(input, intent, taskType)
}

type GCATask string

const (
	TaskInsight             GCATask = "insight"
	TaskChat                GCATask = "chat"
	TaskPrune               GCATask = "prune"
	TaskSummary             GCATask = "summary"
	TaskNarrative           GCATask = "narrative"
	TaskResolveSymbol       GCATask = "resolve_symbol"
	TaskPathEndpoints       GCATask = "path_endpoints"
	TaskDatalog             GCATask = "datalog"
	TaskPathNarrative       GCATask = "path_narrative"
	TaskSmartSearchAnalysis GCATask = "smart_search_analysis"
	TaskMultiFileSummary    GCATask = "multi_file_summary"
	TaskRefactor            GCATask = "refactor"
	TaskTestGeneration      GCATask = "test_generation"
	TaskSecurityAudit       GCATask = "security_audit"
	TaskPerformance         GCATask = "performance"
)

type GCAFrame struct {
	*CognitiveFrame

	ProjectID string
	Task      GCATask
	SymbolID  string
	Data      interface{}

	Prompt    string
	Response  string
	ExecError error
}

func NewGCAFrame(projectID, input string, task GCATask) *GCAFrame {
	return &GCAFrame{
		CognitiveFrame: NewCognitiveFrame(
			input,
			IntentStr(task),
			TaskTypeGeneration,
		),
		ProjectID: projectID,
		Task:      task,
	}
}

type Observer interface {
	Observe(ctx context.Context, frame *GCAFrame) error
}

type Orienter interface {
	Orient(ctx context.Context, frame *GCAFrame) error
}

type Decider interface {
	Decide(ctx context.Context, frame *GCAFrame) error
}

type Verifier interface {
	Verify(ctx context.Context, frame *GCAFrame) error
}

type Actor interface {
	Act(ctx context.Context, frame *GCAFrame) error
}

type GCALoop struct {
	observer Observer
	orienter Orienter
	decider  Decider
	verifier Verifier
	actor    Actor
}

func NewGCALoop(observer Observer, orienter Orienter, decider Decider, verifier Verifier, actor Actor) *GCALoop {
	return &GCALoop{
		observer: observer,
		orienter: orienter,
		decider:  decider,
		verifier: verifier,
		actor:    actor,
	}
}

func (l *GCALoop) Run(ctx context.Context, frame *GCAFrame) (*GCAFrame, error) {
	startTime := time.Now()
	defer func() {
		logger.Debug("OODA Loop completed", "duration", time.Since(startTime))
	}()

	phases := []Phase{PhaseObserve, PhaseOrient, PhaseDecide, PhaseVerify, PhaseAct}

	for _, phase := range phases {
		frame.Phase = phase

		var err error
		switch phase {
		case PhaseObserve:
			if l.observer != nil {
				err = l.observer.Observe(ctx, frame)
			}
		case PhaseOrient:
			if l.orienter != nil {
				err = l.orienter.Orient(ctx, frame)
			}
		case PhaseDecide:
			if l.decider != nil {
				err = l.decider.Decide(ctx, frame)
			}
		case PhaseVerify:
			if l.verifier != nil {
				err = l.verifier.Verify(ctx, frame)
			}
		case PhaseAct:
			if l.actor != nil {
				err = l.actor.Act(ctx, frame)
			}
		}

		if err != nil {
			return frame, fmt.Errorf("OODA loop failed at phase %s: %w", phase, err)
		}
	}

	return frame, nil
}

type IntentClassifier struct{}

func (c *IntentClassifier) Classify(input string) GCATask {
	inputLower := strings.ToLower(input)

	if strings.Contains(inputLower, "analyze") || strings.Contains(inputLower, "insight") || strings.Contains(inputLower, "role") {
		return TaskInsight
	}
	if strings.Contains(inputLower, "explain") || strings.Contains(inputLower, "what is") || strings.Contains(inputLower, "how does") {
		return TaskChat
	}
	if strings.Contains(inputLower, "path") || strings.Contains(inputLower, "flow") || strings.Contains(inputLower, "call chain") {
		return TaskNarrative
	}
	if strings.Contains(inputLower, "summary") || strings.Contains(inputLower, "summarize") {
		return TaskSummary
	}
	if strings.Contains(inputLower, "resolve") || strings.Contains(inputLower, "which") {
		return TaskResolveSymbol
	}

	return TaskChat
}

func ExtractPotentialSymbols(query string) []string {
	var symbols []string
	words := strings.Fields(query)
	for _, word := range words {
		word = strings.Trim(word, ".,!?;:\"'()[]{}")
		if len(word) >= 4 && strings.ContainsAny(word, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz._/") {
			symbols = append(symbols, word)
		}
	}
	return symbols
}
