---
type: npm Package
title: "@genkit-ai/ai"
description: "AI abstraction layer providing the generation pipeline, tool definitions, prompt management, retrievers, embedders, evaluators, rerankers, and chat sessions."
resource: "https://www.npmjs.com/package/@genkit-ai/ai"
tags: [genkit, ai-abstractions, generation]
timestamp: 2026-06-19T00:00:00Z
---

The `@genkit-ai/ai` package sits above `@genkit-ai/core` and provides the high-level AI abstractions. Its central class is `GenkitAI`, which the public [`Genkit`](/core/genkit.md) class extends.

## Source Structure

`ai/src/` contains 27 source files and 4 subdirectories:

| File | Purpose |
|---|---|
| `genkit-ai.ts` | `GenkitAI` class — provides `generate()`, `generateStream()`, `embed()`, etc. |
| `generate.ts` | `generate()`, `generateStream()` — the core generation pipeline |
| `generate/action.ts` | Internal generate action helper |
| `generate/middleware.ts` | [Middleware](/concepts/middleware.md) definitions |
| `generate/response.ts` | `GenerateResponse` type |
| `generate/chunk.ts` | `GenerateResponseChunk` for streaming |
| `model.ts` | [Model](/concepts/models.md) type definitions, `modelRef()`, `defineModel()` |
| `tool.ts` | [Tool](/concepts/tools.md) definitions, interrupts |
| `prompt.ts` | [Prompt](/concepts/prompts.md) definitions |
| `retriever.ts` | [Retriever](/concepts/retrievers.md) definitions |
| `embedder.ts` | [Embedder](/concepts/embedders.md) definitions |
| `evaluator.ts` | [Evaluator](/concepts/evaluators.md) definitions |
| `reranker.ts` | [Reranker](/concepts/rerankers.md) definitions |
| `chat.ts` | Chat session management — see [Chat & Sessions](/concepts/chat-sessions.md) |
| `session.ts` | Session state management — see [Chat & Sessions](/concepts/chat-sessions.md) |
| `document.ts` | `Document` data type |
| `message.ts` | `Message` data type |
| `parts.ts` | Content parts (text, media, tool requests/responses) |
| `formats/` | Output format handlers: json, jsonl, text, array, enum |
| `testing/` | Testing utilities including `model-tester.ts` |

## Links

- Implemented in the [@genkit-ai/ai](/core/genkit-ai.md) package
- Built on [@genkit-ai/core](/core/genkit-core.md)
- Provides the [Generate](/concepts/generate.md) pipeline
- Hosts [Chat & Sessions](/concepts/chat-sessions.md)
- Manages [Tools](/concepts/tools.md) and [Prompts](/concepts/prompts.md)
- Defines [Models](/concepts/models.md), [Retrievers](/concepts/retrievers.md), [Embedders](/concepts/embedders.md), [Evaluators](/concepts/evaluators.md), [Rerankers](/concepts/rerankers.md)

## Dependencies

Depends on [@genkit-ai/core](/core/genkit-core.md) for the [registry](/concepts/registry.md), [actions](/concepts/actions.md), [schemas](/concepts/schemas.md), [tracing](/telemetry/tracing.md), and error handling.
