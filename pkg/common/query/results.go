package query

import (
	"fmt"
	"strconv"

	"github.com/duynguyendang/gca/pkg/common/errors"
)

type Result map[string]any

func (r Result) GetString(key string) (string, error) {
	v, ok := r[key].(string)
	if !ok {
		return "", fmt.Errorf("%w: expected string for key %q, got %T", errors.ErrQueryParseFailed, key, r[key])
	}
	return v, nil
}

func (r Result) GetInt(key string) (int, error) {
	switch v := r[key].(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		return int(v), nil
	case string:
		i, err := strconv.Atoi(v)
		if err != nil {
			return 0, fmt.Errorf("%w: cannot parse %q as int: %v", errors.ErrQueryParseFailed, v, err)
		}
		return i, nil
	default:
		return 0, fmt.Errorf("%w: expected int for key %q, got %T", errors.ErrQueryParseFailed, key, r[key])
	}
}

func (r Result) GetFloat(key string) (float64, error) {
	switch v := r[key].(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case string:
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, fmt.Errorf("%w: cannot parse %q as float: %v", errors.ErrQueryParseFailed, v, err)
		}
		return f, nil
	default:
		return 0, fmt.Errorf("%w: expected float for key %q, got %T", errors.ErrQueryParseFailed, key, r[key])
	}
}

func (r Result) MustGetString(key string) string {
	v, err := r.GetString(key)
	if err != nil {
		panic(err)
	}
	return v
}

func (r Result) MustGetInt(key string) int {
	v, err := r.GetInt(key)
	if err != nil {
		panic(err)
	}
	return v
}

func (r Result) MustGetFloat(key string) float64 {
	v, err := r.GetFloat(key)
	if err != nil {
		panic(err)
	}
	return v
}

type SmellResult struct {
	Subject   string
	Object    string
	Severity  string
	SmellType string
	Detail    string
}

func ParseSmellResult(r Result) (*SmellResult, error) {
	subject, err := r.GetString("Subject")
	if err != nil {
		return nil, fmt.Errorf("smell result missing subject: %w", err)
	}

	object, err := r.GetString("Object")
	if err != nil {
		return nil, fmt.Errorf("smell result missing object: %w", err)
	}

	result := &SmellResult{
		Subject:  subject,
		Object:   object,
		Severity: "Low",
	}

	if len(object) > 15 && object[:15] == "circular_dependency" {
		result.SmellType = "Circular Dependency"
		result.Severity = "High"
		result.Detail = object[16:]
	} else if len(object) > 8 && object[:8] == "god_file" {
		result.SmellType = "God File"
		result.Severity = "Medium"
		result.Detail = object[9:]
	} else if len(object) > 16 && object[:16] == "layer_violation" {
		result.SmellType = "Layer Violation"
		result.Severity = "Medium"
		result.Detail = object[17:]
	} else {
		result.SmellType = object
		result.Severity = "Low"
		result.Detail = object
	}

	return result, nil
}

type HubResult struct {
	Subject string
	Score   int
}

func ParseHubResult(r Result) (*HubResult, error) {
	subject, err := r.GetString("Subject")
	if err != nil {
		return nil, fmt.Errorf("hub result missing subject: %w", err)
	}

	score, err := r.GetInt("Score")
	if err != nil {
		return nil, fmt.Errorf("hub result missing score: %w", err)
	}

	return &HubResult{
		Subject: subject,
		Score:   score,
	}, nil
}

type EntryPointResult struct {
	Subject string
}

func ParseEntryPointResult(r Result) (*EntryPointResult, error) {
	subject, err := r.GetString("Subject")
	if err != nil {
		return nil, fmt.Errorf("entry point result missing subject: %w", err)
	}

	return &EntryPointResult{Subject: subject}, nil
}

type CentralityResult struct {
	Symbol    string
	InDegree  int
	OutDegree int
	Score     float64
}

func ParseCentralityResult(r Result) (*CentralityResult, error) {
	symbol, err := r.GetString("Symbol")
	if err != nil {
		symbol, _ = r.GetString("subject")
	}

	inDegree, _ := r.GetInt("in_degree")
	outDegree, _ := r.GetInt("out_degree")
	score, _ := r.GetFloat("score")

	return &CentralityResult{
		Symbol:    symbol,
		InDegree:  inDegree,
		OutDegree: outDegree,
		Score:     score,
	}, nil
}

func ParseResults[T any](results []map[string]any, parser func(Result) (*T, error)) ([]*T, []error) {
	parsed := make([]*T, 0, len(results))
	errs := make([]error, 0)

	for i, r := range results {
		result, err := parser(r)
		if err != nil {
			errs = append(errs, fmt.Errorf("result[%d]: %w", i, err))
			continue
		}
		parsed = append(parsed, result)
	}

	return parsed, errs
}
