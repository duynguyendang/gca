package errors

import (
	"errors"
	"fmt"
	"net/http"
)

// Common sentinel errors
var (
	ErrInvalidInput = errors.New("invalid input")
	ErrNotFound     = errors.New("not found")
	ErrInternal     = errors.New("internal error")
	ErrUnauthorized = errors.New("unauthorized")
)

// AppError represents an application-specific error with an HTTP status code.
type AppError struct {
	Code    int
	Message string
	Err     error
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
	}
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

	// Default to internal server error
	return NewAppError(http.StatusInternalServerError, "Internal server error", err)
}
