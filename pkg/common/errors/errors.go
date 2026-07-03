package errors

import (
	"errors"
	"fmt"
	"net/http"
)

// Common sentinel errors
var (
	ErrInvalidInput       = errors.New("invalid input")
	ErrNotFound           = errors.New("not found")
	ErrInternal           = errors.New("internal error")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrForbidden          = errors.New("forbidden")
	ErrConflict           = errors.New("conflict")
	ErrTimeout            = errors.New("timeout")
	ErrRateLimited        = errors.New("rate limited")
	ErrServiceUnavailable = errors.New("service unavailable")
)

// Graph-specific errors
var (
	ErrGraphNotFound         = errors.New("graph not found")
	ErrGraphInvalidQuery     = errors.New("invalid graph query")
	ErrGraphHydrationFailed  = errors.New("graph hydration failed")
	ErrGraphClusteringFailed = errors.New("graph clustering failed")
	ErrGraphPathNotFound     = errors.New("graph path not found")
)

// Store-specific errors
var (
	ErrStoreNotFound    = errors.New("store not found")
	ErrStoreUnavailable = errors.New("store unavailable")
	ErrStoreCorrupted   = errors.New("store corrupted")
)

// Query-specific errors
var (
	ErrQueryParseFailed     = errors.New("query parse failed")
	ErrQueryExecutionFailed = errors.New("query execution failed")
	ErrQueryTimeout         = errors.New("query timeout")
)

// Ingestion-specific errors
var (
	ErrIngestionFailed = errors.New("ingestion failed")
	ErrInvalidFileType = errors.New("invalid file type")
	ErrFileTooLarge    = errors.New("file too large")
)

// AI/LLM-specific errors
var (
	ErrAIRequestFailed   = errors.New("AI request failed")
	ErrAIResponseInvalid = errors.New("AI response invalid")
	ErrEmbeddingFailed   = errors.New("embedding failed")
)

// AppError represents an application-specific error with an HTTP status code.
type AppError struct {
	Code    int
	Message string
	Err     error
	Details map[string]interface{}
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *AppError) Unwrap() error {
	return e.Err
}

// NewAppError creates a new AppError.
func NewAppError(code int, message string, err error) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Err:     err,
		Details: make(map[string]interface{}),
	}
}

type errorMapping struct {
	err     error
	code    int
	message string
}

var errorMappings = []errorMapping{
	{ErrInvalidInput,       http.StatusBadRequest,          "Invalid request"},
	{ErrNotFound,           http.StatusNotFound,            "Resource not found"},
	{ErrUnauthorized,       http.StatusUnauthorized,        "Unauthorized"},
	{ErrForbidden,          http.StatusForbidden,           "Forbidden"},
	{ErrConflict,           http.StatusConflict,            "Conflict"},
	{ErrTimeout,            http.StatusRequestTimeout,      "Request timeout"},
	{ErrRateLimited,        http.StatusTooManyRequests,     "Rate limited"},
	{ErrServiceUnavailable, http.StatusServiceUnavailable,  "Service unavailable"},
	{ErrGraphNotFound,           http.StatusNotFound,              "Graph not found"},
	{ErrGraphInvalidQuery,       http.StatusBadRequest,            "Invalid graph query"},
	{ErrGraphHydrationFailed,    http.StatusInternalServerError,   "Graph hydration failed"},
	{ErrGraphClusteringFailed,   http.StatusInternalServerError,   "Graph clustering failed"},
	{ErrGraphPathNotFound,       http.StatusNotFound,              "Graph path not found"},
	{ErrStoreNotFound,           http.StatusNotFound,              "Store not found"},
	{ErrStoreUnavailable,        http.StatusServiceUnavailable,    "Store unavailable"},
	{ErrStoreCorrupted,          http.StatusInternalServerError,   "Store corrupted"},
	{ErrQueryParseFailed,        http.StatusBadRequest,            "Query parse failed"},
	{ErrQueryExecutionFailed,    http.StatusInternalServerError,   "Query execution failed"},
	{ErrQueryTimeout,            http.StatusRequestTimeout,        "Query timeout"},
	{ErrIngestionFailed,         http.StatusInternalServerError,   "Ingestion failed"},
	{ErrInvalidFileType,         http.StatusBadRequest,            "Invalid file type"},
	{ErrFileTooLarge,            http.StatusRequestEntityTooLarge, "File too large"},
	{ErrAIRequestFailed,         http.StatusBadGateway,            "AI request failed"},
	{ErrAIResponseInvalid,       http.StatusBadGateway,            "AI response invalid"},
	{ErrEmbeddingFailed,         http.StatusBadGateway,            "Embedding failed"},
}

// MapError maps a common error to an AppError with an appropriate HTTP status code.
func MapError(err error) *AppError {
	if err == nil {
		return nil
	}

	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr
	}

	for _, m := range errorMappings {
		if errors.Is(err, m.err) {
			return NewAppError(m.code, m.message, err)
		}
	}

	return NewAppError(http.StatusInternalServerError, "Internal server error", err)
}

// IsNotFound checks if the error is a not found error.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound) || errors.Is(err, ErrGraphNotFound) || errors.Is(err, ErrStoreNotFound)
}

// IsInvalidInput checks if the error is an invalid input error.
func IsInvalidInput(err error) bool {
	return errors.Is(err, ErrInvalidInput) || errors.Is(err, ErrGraphInvalidQuery) || errors.Is(err, ErrQueryParseFailed)
}

// IsInternal checks if the error is an internal error.
func IsInternal(err error) bool {
	return errors.Is(err, ErrInternal) || errors.Is(err, ErrGraphHydrationFailed) ||
		errors.Is(err, ErrGraphClusteringFailed) || errors.Is(err, ErrQueryExecutionFailed) ||
		errors.Is(err, ErrIngestionFailed) || errors.Is(err, ErrStoreCorrupted)
}

// IsTimeout checks if the error is a timeout error.
func IsTimeout(err error) bool {
	return errors.Is(err, ErrTimeout) || errors.Is(err, ErrQueryTimeout)
}

// IsServiceUnavailable checks if the error is a service unavailable error.
func IsServiceUnavailable(err error) bool {
	return errors.Is(err, ErrServiceUnavailable) || errors.Is(err, ErrStoreUnavailable)
}
