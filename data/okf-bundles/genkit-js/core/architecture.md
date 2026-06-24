---
type: Reference
title: Genkit JS Architecture
description: Dependency hierarchy and layering of the Genkit JS framework packages.
tags: [genkit, architecture, layering]
timestamp: 2026-06-19T00:00:00Z
---

## Layer Diagram

```
genkit (npm package, public entry point)
  └── @genkit-ai/ai  (AI abstractions: generate, tools, prompts, chat)
        └── @genkit-ai/core  (foundation: registry, actions, flows, telemetry)
              └── Node.js runtime (async context, tracing)
```

## How It Fits Together

1. **`@genkit-ai/core`** provides the substrate: a [registry](/concepts/registry.md) that holds all registrable objects, an [action](/concepts/actions.md) abstraction for wrapping any function as a traced/observable unit, a [flow](/concepts/flows.md) system for strongly-typed observable workflows, a [plugin](/concepts/plugins.md) system for loading external capabilities, OpenTelemetry [tracing](/telemetry/tracing.md), and a dev-mode [Reflection API](/concepts/reflection-api.md).

2. **`@genkit-ai/ai`** builds on core with AI-specific types and operations: [model](/concepts/models.md) definitions, the [generation pipeline](/concepts/generate.md), [tools](/concepts/tools.md), [prompts](/concepts/prompts.md), [retrievers](/concepts/retrievers.md), [embedders](/concepts/embedders.md), [evaluators](/concepts/evaluators.md), [rerankers](/concepts/rerankers.md), chat/session management, and output format handlers.

3. **`genkit`** (the public package) re-exports everything from `@genkit-ai/ai` and adds the [`Genkit`](/core/genkit.md) class that wires plugins, starts the reflection server, and manages the full lifecycle.
