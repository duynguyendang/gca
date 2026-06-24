---
type: Concept
title: Generate
description: The core generation pipeline — resolves a model, applies middleware, resolves tools, and formats output.
tags: [genkit, generate, generation, core-concept]
timestamp: 2026-06-19T00:00:00Z
---

The **generate** pipeline is the central operation in Genkit. It takes a prompt (and optionally tools, config, and middleware) and produces a model response.

## Source

- [generate](ai/src/generate.ts#generate) — Core generation function
- [generateStream](ai/src/generate.ts#generateStream) — Streaming generation

`ai/src/generate/` — Internal pipeline modules:

| File | Purpose |
|---|---|
| `action.ts` | Wraps generation as an [Action](/concepts/actions.md) |
| `middleware.ts` | [Middleware](/concepts/middleware.md) definitions for generation |
| `response.ts` | `GenerateResponse` type |
| `chunk.ts` | `GenerateResponseChunk` for streaming |
| `resolve-tool-requests.ts` | Automatic tool call resolution |

## Pipeline Flow

1. Resolve the model from the [registry](/concepts/registry.md)
2. Apply [middleware](/concepts/middleware.md) chain (retry, fallback, etc.)
3. Apply output format constraints (json, jsonl, text, array, enum)
4. Resolve [tool](/concepts/tools.md) requests if the model calls tools
5. Stream response chunks via `generateStream()`

## API

```typescript
const response = await ai.generate({
  model: 'gemini-2.5-flash',     // Model reference
  prompt: 'Tell me a joke',       // Text prompt
  tools: [myTool],                // Optional tools
  config: { temperature: 0.7 },   // Optional model config
});

// Streaming
const { stream, response } = await ai.generateStream({
  model: 'gemini-2.5-flash',
  prompt: 'Tell me a story',
});
```

## Links

- Uses [Models](/concepts/models.md)
- Calls [Tools](/concepts/tools.md) automatically
- Configured via [Middleware](/concepts/middleware.md)
- Validated with [Schemas](/concepts/schemas.md)
- Consumed by [Chat](/concepts/chat-sessions.md)
- Implemented in [@genkit-ai/ai](/core/genkit-ai.md)
