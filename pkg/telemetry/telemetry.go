package telemetry

import (
	"fmt"
	"sync"
	"time"

	"github.com/duynguyendang/gca/pkg/logger"
	"github.com/duynguyendang/meb"
)

// LoggerSink implements meb.TelemetrySink and logs telemetry events.
type LoggerSink struct {
	mu    sync.Mutex
	count int64
}

// NewLoggerSink creates a new LoggerSink.
func NewLoggerSink() *LoggerSink {
	return &LoggerSink{}
}

// OnEvent handles incoming telemetry events.
func (s *LoggerSink) OnEvent(event meb.TelemetryEvent) {
	s.mu.Lock()
	s.count++
	count := s.count
	s.mu.Unlock()

	switch event.Type {
	case "circuit_state_change":
		s.logCircuitStateChange(event, count)
	case "gc_failure":
		s.logGCFailure(event, count)
	case "retention":
		s.logRetention(event, count)
	case "wal_clear_failed":
		s.logWALIssue(event, count)
	case "deprecated_cleanup", "deprecated_cleanup_failed":
		s.logDeprecatedCleanup(event, count)
	default:
		s.logGeneric(event, count)
	}
}

// Count returns the total number of events processed.
func (s *LoggerSink) Count() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.count
}

func (s *LoggerSink) logCircuitStateChange(event meb.TelemetryEvent, count int64) {
	state := event.Data["state"]
	reason := event.Data["reason"]
	logger.Info("Circuit breaker state changed", "count", count, "state", state, "reason", reason)
}

func (s *LoggerSink) logGCFailure(event meb.TelemetryEvent, count int64) {
	err := event.Data["error"]
	logger.Warn("GC failure", "count", count, "error", err)
}

func (s *LoggerSink) logRetention(event meb.TelemetryEvent, count int64) {
	action := event.Data["action"]
	deleted := event.Data["deleted"]
	logger.Info("Retention event", "count", count, "action", action, "deleted", deleted)
}

func (s *LoggerSink) logWALIssue(event meb.TelemetryEvent, count int64) {
	err := event.Data["error"]
	logger.Warn("WAL issue", "count", count, "error", err)
}

func (s *LoggerSink) logDeprecatedCleanup(event meb.TelemetryEvent, count int64) {
	err := event.Data["error"]
	logger.Warn("Deprecated cleanup event", "count", count, "error", err, "type", event.Type)
}

func (s *LoggerSink) logGeneric(event meb.TelemetryEvent, count int64) {
	logger.Debug("Telemetry event", "count", count, "type", event.Type, "data", formatData(event.Data))
}

func formatData(data map[string]any) string {
	if len(data) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(data))
	for k, v := range data {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	return "{" + join(parts, ", ") + "}"
}

func join(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += sep + parts[i]
	}
	return result
}

// MetricsSink implements meb.TelemetrySink and tracks metrics.
type MetricsSink struct {
	mu         sync.Mutex
	eventCount map[string]int64
	startTime  time.Time
	lastReport time.Time
	reportFreq time.Duration
}

// NewMetricsSink creates a new MetricsSink that reports metrics periodically.
func NewMetricsSink(reportFreq time.Duration) *MetricsSink {
	now := time.Now()
	return &MetricsSink{
		eventCount: make(map[string]int64),
		startTime:  now,
		lastReport: now,
		reportFreq: reportFreq,
	}
}

// OnEvent handles incoming telemetry events and tracks metrics.
func (s *MetricsSink) OnEvent(event meb.TelemetryEvent) {
	s.mu.Lock()
	s.eventCount[event.Type]++

	shouldReport := time.Since(s.lastReport) >= s.reportFreq
	if shouldReport {
		s.lastReport = time.Now()
		s.reportLocked()
	}
	s.mu.Unlock()
}

func (s *MetricsSink) reportLocked() {
	uptime := time.Since(s.startTime)
	total := int64(0)
	for _, c := range s.eventCount {
		total += c
	}

	logger.Info("Metrics report", "uptime", uptime.Truncate(time.Second), "totalEvents", total)
	for eventType, count := range s.eventCount {
		logger.Debug("Event count", "type", eventType, "count", count)
	}
}

// Snapshot returns a copy of the current event counts.
func (s *MetricsSink) Snapshot() map[string]int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot := make(map[string]int64, len(s.eventCount))
	for k, v := range s.eventCount {
		snapshot[k] = v
	}
	return snapshot
}
