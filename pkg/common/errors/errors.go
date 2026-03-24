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

// NewAppErrorWithDetails creates a new AppError with additional details.
func NewAppErrorWithDetails(code int, message string, err error, details map[string]interface{}) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Err:     err,
		Details: details,
	}
}

// WithDetail adds a detail to the error.
func (e *AppError) WithDetail(key string, value interface{}) *AppError {
	if e.Details == nil {
		e.Details = make(map[string]interface{})
	}
	e.Details[key] = value
	return e
}

// MapError maps a common error to an AppError with an appropriate HTTP status code.
func MapError(err error) *AppError {
	if err == nil {
		return nil
	}

	// Check for existing AppError
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr
	}

	// Map sentinel errors
	if errors.Is(err, ErrInvalidInput) {
		return NewAppError(http.StatusBadRequest, "Invalid request", err)
	}
	if errors.Is(err, ErrNotFound) {
		return NewAppError(http.StatusNotFound, "Resource not found", err)
	}
	if errors.Is(err, ErrUnauthorized) {
		return NewAppError(http.StatusUnauthorized, "Unauthorized", err)
	}
	if errors.Is(err, ErrForbidden) {
		return NewAppError(http.StatusForbidden, "Forbidden", err)
	}
	if errors.Is(err, ErrConflict) {
		return NewAppError(http.StatusConflict, "Conflict", err)
	}
	if errors.Is(err, ErrTimeout) {
		return NewAppError(http.StatusRequestTimeout, "Request timeout", err)
	}
	if errors.Is(err, ErrRateLimited) {
		return NewAppError(http.StatusTooManyRequests, "Rate limited", err)
	}
	if errors.Is(err, ErrServiceUnavailable) {
		return NewAppError(http.StatusServiceUnavailable, "Service unavailable", err)
	}

	// Map graph-specific errors
	if errors.Is(err, ErrGraphNotFound) {
		return NewAppError(http.StatusNotFound, "Graph not found", err)
	}
	if errors.Is(err, ErrGraphInvalidQuery) {
		return NewAppError(http.StatusBadRequest, "Invalid graph query", err)
	}
	if errors.Is(err, ErrGraphHydrationFailed) {
		return NewAppError(http.StatusInternalServerError, "Graph hydration failed", err)
	}
	if errors.Is(err, ErrGraphClusteringFailed) {
		return NewAppError(http.StatusInternalServerError, "Graph clustering failed", err)
	}
	if errors.Is(err, ErrGraphPathNotFound) {
		return NewAppError(http.StatusNotFound, "Graph path not found", err)
	}

	// Map store-specific errors
	if errors.Is(err, ErrStoreNotFound) {
		return NewAppError(http.StatusNotFound, "Store not found", err)
	}
	if errors.Is(err, ErrStoreUnavailable) {
		return NewAppError(http.StatusServiceUnavailable, "Store unavailable", err)
	}
	if errors.Is(err, ErrStoreCorrupted) {
		return NewAppError(http.StatusInternalServerError, "Store corrupted", err)
	}

	// Map query-specific errors
	if errors.Is(err, ErrQueryParseFailed) {
		return NewAppError(http.StatusBadRequest, "Query parse failed", err)
	}
	if errors.Is(err, ErrQueryExecutionFailed) {
		return NewAppError(http.StatusInternalServerError, "Query execution failed", err)
	}
	if errors.Is(err, ErrQueryTimeout) {
		return NewAppError(http.StatusRequestTimeout, "Query timeout", err)
	}

	// Map ingestion-specific errors
	if errors.Is(err, ErrIngestionFailed) {
		return NewAppError(http.StatusInternalServerError, "Ingestion failed", err)
	}
	if errors.Is(err, ErrInvalidFileType) {
		return NewAppError(http.StatusBadRequest, "Invalid file type", err)
	}
	if errors.Is(err, ErrFileTooLarge) {
		return NewAppError(http.StatusRequestEntityTooLarge, "File too large", err)
	}

	// Map AI/LLM-specific errors
	if errors.Is(err, ErrAIRequestFailed) {
		return NewAppError(http.StatusBadGateway, "AI request failed", err)
	}
	if errors.Is(err, ErrAIResponseInvalid) {
		return NewAppError(http.StatusBadGateway, "AI response invalid", err)
	}
	if errors.Is(err, ErrEmbeddingFailed) {
		return NewAppError(http.StatusBadGateway, "Embedding failed", err)
	}

	// Default to internal server error
	return NewAppError(http.StatusInternalServerError, "Internal server error", err)
}

// WrapError wraps an error with additional context.
func WrapError(err error, message string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}

// WrapErrorf wraps an error with formatted message.
func WrapErrorf(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), err)
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
