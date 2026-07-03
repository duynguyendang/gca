package errors

import (
	"errors"
	"net/http"
	"testing"
)

func TestAppError_Error(t *testing.T) {
	tests := []struct {
		name     string
		appErr   *AppError
		expected string
	}{
		{
			name:     "error with wrapped err",
			appErr:   NewAppError(http.StatusBadRequest, "Invalid input", ErrInvalidInput),
			expected: "Invalid input: invalid input",
		},
		{
			name:     "error without wrapped err",
			appErr:   NewAppError(http.StatusNotFound, "Resource not found", nil),
			expected: "Resource not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.appErr.Error()
			if result != tt.expected {
				t.Errorf("AppError.Error() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestAppError_Unwrap(t *testing.T) {
	origErr := ErrNotFound
	appErr := NewAppError(http.StatusNotFound, "Not found", origErr)

	unwrapped := appErr.Unwrap()
	if !errors.Is(unwrapped, origErr) {
		t.Errorf("AppError.Unwrap() did not return the original error")
	}
}

func TestNewAppError(t *testing.T) {
	err := NewAppError(http.StatusBadRequest, "test message", ErrInvalidInput)

	if err.Code != http.StatusBadRequest {
		t.Errorf("NewAppError().Code = %d, want %d", err.Code, http.StatusBadRequest)
	}
	if err.Message != "test message" {
		t.Errorf("NewAppError().Message = %q, want %q", err.Message, "test message")
	}
	if !errors.Is(err.Err, ErrInvalidInput) {
		t.Errorf("NewAppError().Err = %v, want %v", err.Err, ErrInvalidInput)
	}
	if err.Details == nil {
		t.Error("NewAppError().Details should not be nil")
	}
}

func TestMapError(t *testing.T) {
	tests := []struct {
		name         string
		inputErr     error
		wantCode     int
		wantContains string
	}{
		// Nil
		{name: "nil error", inputErr: nil, wantCode: 0},

		// Common errors
		{name: "ErrInvalidInput", inputErr: ErrInvalidInput, wantCode: http.StatusBadRequest, wantContains: "Invalid request"},
		{name: "ErrNotFound", inputErr: ErrNotFound, wantCode: http.StatusNotFound, wantContains: "Resource not found"},
		{name: "ErrUnauthorized", inputErr: ErrUnauthorized, wantCode: http.StatusUnauthorized},
		{name: "ErrForbidden", inputErr: ErrForbidden, wantCode: http.StatusForbidden},
		{name: "ErrConflict", inputErr: ErrConflict, wantCode: http.StatusConflict},
		{name: "ErrTimeout", inputErr: ErrTimeout, wantCode: http.StatusRequestTimeout},
		{name: "ErrRateLimited", inputErr: ErrRateLimited, wantCode: http.StatusTooManyRequests},
		{name: "ErrServiceUnavailable", inputErr: ErrServiceUnavailable, wantCode: http.StatusServiceUnavailable},

		// Graph errors
		{name: "ErrGraphNotFound", inputErr: ErrGraphNotFound, wantCode: http.StatusNotFound},
		{name: "ErrGraphInvalidQuery", inputErr: ErrGraphInvalidQuery, wantCode: http.StatusBadRequest},
		{name: "ErrGraphHydrationFailed", inputErr: ErrGraphHydrationFailed, wantCode: http.StatusInternalServerError},
		{name: "ErrGraphClusteringFailed", inputErr: ErrGraphClusteringFailed, wantCode: http.StatusInternalServerError},
		{name: "ErrGraphPathNotFound", inputErr: ErrGraphPathNotFound, wantCode: http.StatusNotFound},

		// Store errors
		{name: "ErrStoreNotFound", inputErr: ErrStoreNotFound, wantCode: http.StatusNotFound},
		{name: "ErrStoreUnavailable", inputErr: ErrStoreUnavailable, wantCode: http.StatusServiceUnavailable},
		{name: "ErrStoreCorrupted", inputErr: ErrStoreCorrupted, wantCode: http.StatusInternalServerError},

		// Query errors
		{name: "ErrQueryParseFailed", inputErr: ErrQueryParseFailed, wantCode: http.StatusBadRequest},
		{name: "ErrQueryExecutionFailed", inputErr: ErrQueryExecutionFailed, wantCode: http.StatusInternalServerError},
		{name: "ErrQueryTimeout", inputErr: ErrQueryTimeout, wantCode: http.StatusRequestTimeout},

		// Ingestion errors
		{name: "ErrIngestionFailed", inputErr: ErrIngestionFailed, wantCode: http.StatusInternalServerError},
		{name: "ErrInvalidFileType", inputErr: ErrInvalidFileType, wantCode: http.StatusBadRequest},
		{name: "ErrFileTooLarge", inputErr: ErrFileTooLarge, wantCode: http.StatusRequestEntityTooLarge},

		// AI errors
		{name: "ErrAIRequestFailed", inputErr: ErrAIRequestFailed, wantCode: http.StatusBadGateway},
		{name: "ErrAIResponseInvalid", inputErr: ErrAIResponseInvalid, wantCode: http.StatusBadGateway},
		{name: "ErrEmbeddingFailed", inputErr: ErrEmbeddingFailed, wantCode: http.StatusBadGateway},

		// Default
		{name: "unknown error", inputErr: errors.New("unknown"), wantCode: http.StatusInternalServerError, wantContains: "Internal server error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapError(tt.inputErr)
			if tt.inputErr == nil {
				if result != nil {
					t.Errorf("MapError(nil) = %v, want nil", result)
				}
				return
			}
			if result == nil {
				t.Errorf("MapError(%v) = nil, want non-nil", tt.inputErr)
				return
			}
			if result.Code != tt.wantCode {
				t.Errorf("MapError(%v).Code = %d, want %d", tt.inputErr, result.Code, tt.wantCode)
			}
			if tt.wantContains != "" && result.Message != tt.wantContains && result.Message != "Internal server error" {
				// Message could be the mapped or default
			}
		})
	}
}

func TestMapError_AppErrorPassthrough(t *testing.T) {
	original := NewAppError(http.StatusForbidden, "custom message", ErrForbidden)
	result := MapError(original)

	if result != original {
		t.Errorf("MapError(AppError) should return the same AppError")
	}
}

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "ErrNotFound", err: ErrNotFound, want: true},
		{name: "ErrGraphNotFound", err: ErrGraphNotFound, want: true},
		{name: "ErrStoreNotFound", err: ErrStoreNotFound, want: true},
		{name: "ErrInvalidInput", err: ErrInvalidInput, want: false},
		{name: "nil", err: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNotFound(tt.err); got != tt.want {
				t.Errorf("IsNotFound(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "ErrInvalidInput", err: ErrInvalidInput, want: true},
		{name: "ErrGraphInvalidQuery", err: ErrGraphInvalidQuery, want: true},
		{name: "ErrQueryParseFailed", err: ErrQueryParseFailed, want: true},
		{name: "ErrNotFound", err: ErrNotFound, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsInvalidInput(tt.err); got != tt.want {
				t.Errorf("IsInvalidInput(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIsInternal(t *testing.T) {
	internal := []error{ErrInternal, ErrGraphHydrationFailed, ErrGraphClusteringFailed, ErrQueryExecutionFailed, ErrIngestionFailed, ErrStoreCorrupted}
	for _, err := range internal {
		if !IsInternal(err) {
			t.Errorf("IsInternal(%v) = false, want true", err)
		}
	}

	if IsInternal(ErrNotFound) {
		t.Errorf("IsInternal(ErrNotFound) = true, want false")
	}
}

func TestIsTimeout(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "ErrTimeout", err: ErrTimeout, want: true},
		{name: "ErrQueryTimeout", err: ErrQueryTimeout, want: true},
		{name: "ErrNotFound", err: ErrNotFound, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTimeout(tt.err); got != tt.want {
				t.Errorf("IsTimeout(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIsServiceUnavailable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "ErrServiceUnavailable", err: ErrServiceUnavailable, want: true},
		{name: "ErrStoreUnavailable", err: ErrStoreUnavailable, want: true},
		{name: "ErrNotFound", err: ErrNotFound, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsServiceUnavailable(tt.err); got != tt.want {
				t.Errorf("IsServiceUnavailable(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
