package logger

import (
	"log/slog"
	"os"
	"strings"
	"sync"
)

// Level represents a log level
type Level = slog.Level

const (
	// LevelDebug is for detailed debugging information
	LevelDebug Level = slog.LevelDebug
	// LevelInfo is for general operational information
	LevelInfo Level = slog.LevelInfo
	// LevelWarn is for warning conditions
	LevelWarn Level = slog.LevelWarn
	// LevelError is for error conditions
	LevelError Level = slog.LevelError
)

var (
	defaultLogger *slog.Logger
	level         Level
	mu            sync.RWMutex
)

func init() {
	// Initialize with INFO level by default
	level = LevelInfo
	defaultLogger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	}))
}

// SetLevel sets the global log level
func SetLevel(l Level) {
	mu.Lock()
	defer mu.Unlock()
	level = l
	defaultLogger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: l,
	}))
}

// SetLevelFromString sets the log level from a string (DEBUG, INFO, WARN, ERROR)
func SetLevelFromString(s string) {
	switch strings.ToUpper(s) {
	case "DEBUG":
		SetLevel(LevelDebug)
	case "INFO":
		SetLevel(LevelInfo)
	case "WARN", "WARNING":
		SetLevel(LevelWarn)
	case "ERROR":
		SetLevel(LevelError)
	default:
		SetLevel(LevelInfo)
	}
}

// GetLevel returns the current log level
func GetLevel() Level {
	mu.RLock()
	defer mu.RUnlock()
	return level
}

// GetLogger returns the current default logger
func GetLogger() *slog.Logger {
	mu.RLock()
	defer mu.RUnlock()
	return defaultLogger
}

// Debug logs at DEBUG level
func Debug(msg string, args ...any) {
	GetLogger().Debug(msg, args...)
}

// Debugf logs at DEBUG level with formatted message
func Debugf(msg string, args ...any) {
	GetLogger().Debug(msg, args...)
}

// Info logs at INFO level
func Info(msg string, args ...any) {
	GetLogger().Info(msg, args...)
}

// Infof logs at INFO level with formatted message
func Infof(msg string, args ...any) {
	GetLogger().Info(msg, args...)
}

// Warn logs at WARN level
func Warn(msg string, args ...any) {
	GetLogger().Warn(msg, args...)
}

// Warnf logs at WARN level with formatted message
func Warnf(msg string, args ...any) {
	GetLogger().Warn(msg, args...)
}

// Error logs at ERROR level
func Error(msg string, args ...any) {
	GetLogger().Error(msg, args...)
}

// Errorf logs at ERROR level with formatted message
func Errorf(msg string, args ...any) {
	GetLogger().Error(msg, args...)
}

// With returns a logger with additional attributes
func With(args ...any) *slog.Logger {
	return GetLogger().With(args...)
}
