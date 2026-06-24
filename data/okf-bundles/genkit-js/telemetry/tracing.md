---
type: Concept
title: Tracing
description: OpenTelemetry-based distributed tracing for Genkit actions, flows, and generation. Integrates with Cloud Trace, Cloud Monitoring, and custom exporters.
tags: [genkit, tracing, telemetry, opentelemetry, observability]
timestamp: 2026-06-19T00:00:00Z
---

Genkit has first-class **OpenTelemetry** tracing built in at every level. Every [Action](/concepts/actions.md) invocation, [Flow](/concepts/flows.md) execution, and [Generate](/concepts/generate.md) call is automatically traced.

## Source

- [Tracing](core/src/tracing.ts#Tracing) — OpenTelemetry tracing entry point
- [TraceExporter](core/src/tracing/exporter.ts#TraceExporter) — Trace export interface
- [Instrumentation](core/src/tracing/instrumentation.ts#Instrumentation) — Auto-instrumentation

## Key Features

- **Automatic** — Every action, flow, and generate call is traced without developer effort
- **OpenTelemetry** — Standard OTLP format, compatible with any OpenTelemetry collector
- **Cloud Monitoring** — `@genkit-ai/google-cloud` plugin exports traces to Google Cloud Trace
- **Realtime spans** — `RealtimeSpanProcessor` feeds the Dev UI during development
- **Custom exporters** — Implement your own trace export target

## Links

- Defined in [@genkit-ai/core](/core/genkit-core.md)
- Monitored via [@genkit-ai/google-cloud](/plugins/index.md)
