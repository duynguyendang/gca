---
type: npm Package
title: "@genkit-ai/core"
description: "Foundation layer providing the registry, action system, flow engine, plugin system, schema utilities, OpenTelemetry tracing, and the Reflection API for the Genkit Developer UI."
resource: "https://www.npmjs.com/package/@genkit-ai/core"
tags: [genkit, foundation, registry, actions, flows, telemetry]
timestamp: 2026-06-19T00:00:00Z
---

`@genkit-ai/core` is the foundational package. Every other package depends on it.

## Source Structure

`core/src/` contains 24 source files and a `tracing/` directory:

| File | Purpose |
|---|---|
| `registry.ts` | Central [registry](/concepts/registry.md) for all actions, plugins, schemas |
| `action.ts` | [Action](/concepts/actions.md) abstraction — runnable, observable, traced unit |
| `flow.ts` | [Flow](/concepts/flows.md) definitions — observable, streamable workflows |
| `plugin.ts` | [Plugin](/concepts/plugins.md) system — `PluginProvider`, `InitializedPlugin` |
| `config.ts` | Runtime configuration management |
| `context.ts` | Action context (API keys, request data) |
| `error.ts` | `GenkitError`, `UserFacingError`, `UnstableApiError` |
| `schema.ts` | Zod + JSON Schema interoperability |
| `tracing.ts` | OpenTelemetry tracing entry point |
| `tracing/exporter.ts` | Trace exporters |
| `tracing/instrumentation.ts` | Auto-instrumentation |
| `tracing/types.ts` | Telemetry type definitions |
| `reflection.ts` | [Reflection API](/concepts/reflection-api.md) — dev server on port 3100 |
| `streaming.ts` | `InMemoryStreamManager`, streaming abstractions |
| `async.ts` | `Channel`, `lazy`, `AsyncTaskQueue` utilities |

## Links

- Provides the [Registry](/concepts/registry.md)
- Defines [Actions](/concepts/actions.md), [Flows](/concepts/flows.md), [Plugins](/concepts/plugins.md)
- Validates with [Schemas](/concepts/schemas.md)
- Serves the [Reflection API](/concepts/reflection-api.md)
- Exports [Tracing](/telemetry/tracing.md)
- Re-exported from [genkit](/core/genkit.md)

## Key Dependencies

- [Zod](https://zod.dev) — Runtime schema validation
- [OpenTelemetry](https://opentelemetry.io) — Distributed tracing and metrics
