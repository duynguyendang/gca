---
type: Concept
title: Actions
description: The fundamental unit of work in Genkit. Every model, tool, flow, retriever, embedder, evaluator, and reranker is an Action.
tags: [genkit, actions, core-concept]
timestamp: 2026-06-19T00:00:00Z
---

An **Action** is a typed, runnable, observable unit of work. Actions are the universal abstraction in Genkit — everything is an action.

## Source

- [Action](core/src/action.ts#Action) — Generic action interface `<I, O, S>`
- [defineAction](core/src/action.ts#defineAction) — Create and register an action
- [defineActionAsync](core/src/action.ts#defineActionAsync) — Create action with async config
- [actionWithMiddleware](core/src/action.ts#actionWithMiddleware) — Wrap action with middleware
- [ActionMetadata](core/src/action.ts#ActionMetadata) — Action metadata interface
- [ActionResult](core/src/action.ts#ActionResult) — Result of an action run with telemetry
- [ActionRunOptions](core/src/action.ts#ActionRunOptions) — Options passed to action execution

## Properties

- **Typed** — Input, output, and stream types enforced at runtime via Zod schemas
- **Observable** — Every invocation is traced via OpenTelemetry
- **Registrable** — Actions are stored in the [registry](/concepts/registry.md) by type and name
- **Streamable** — Support for streaming responses via `streamSchema`
- **Middleware** — Actions can be wrapped with [middleware](/concepts/middleware.md)

## Action Types (16)

The registry supports 16 action types: `model`, `tool`, `flow`, `retriever`, `embedder`, `evaluator`, `reranker`, `util`, `custom`, and more.

## Architecture Position

Actions are the foundational abstraction in Genkit's [architecture](/core/architecture.md). Every higher-level construct (models, tools, flows, etc.) is implemented as an action:

- [Models](/concepts/models.md) — Actions that wrap LLM provider APIs
- [Tools](/concepts/tools.md) — Actions that models can invoke during generation
- [Flows](/concepts/flows.md) — Observable, streamable workflow actions
- [Retrievers](/concepts/retrievers.md) — Actions for document retrieval in RAG
- [Embedders](/concepts/embedders.md) — Actions for text-to-vector conversion
- [Evaluators](/concepts/evaluators.md) — Actions for output quality assessment
- [Rerankers](/concepts/rerankers.md) — Actions for result relevance re-ranking

## Links

- Defined in [@genkit-ai/core](/core/genkit-core.md)
- Registered in the [Registry](/concepts/registry.md)
- Used by [Generate](/concepts/generate.md), [Flows](/concepts/flows.md), [Tools](/concepts/tools.md)
- Traced via [Telemetry](/telemetry/tracing.md)
- Composed with [Middleware](/concepts/middleware.md)
- Position in [Architecture](/core/architecture.md)
