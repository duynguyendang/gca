---
type: npm Package
title: genkit
description: Public entry point for the Genkit JS framework. End-users import from this package.
resource: https://www.npmjs.com/package/genkit
tags: [genkit, entry-point, public-api]
timestamp: 2026-06-19T00:00:00Z
---

The `genkit` package is what end-users install and import. It re-exports all public APIs from `@genkit-ai/ai` and `@genkit-ai/core` and provides the [`Genkit`](genkit/src/genkit.ts#Genkit) class that orchestrates the [registry](/concepts/registry.md), [reflection server](/concepts/reflection-api.md), [plugin](/concepts/plugins.md) initialization, [flow](/concepts/flows.md) definitions, and [tracing](/telemetry/tracing.md).

## Source

- [Genkit](genkit/src/genkit.ts#Genkit) — Main framework facade class (extends [`GenkitAI`](/core/genkit-ai.md))
- [genkit](genkit/src/genkit.ts#genkit) — Factory function

## Key Exports

- `genkit()` — Factory function to create a `Genkit` instance
- `Genkit` class — Main framework facade (extends `GenkitAI`)
- `defineFlow()` / `flow()` — [Flow](/concepts/flows.md) [Action](/concepts/actions.md) definitions
- `defineTool()` — [Tool](/concepts/tools.md) [Action](/concepts/actions.md) definitions
- All model, retriever, embedder, evaluator, reranker, prompt APIs from `@genkit-ai/ai`

## Usage

```typescript
import { genkit } from 'genkit';
import { googleAI } from '@genkit-ai/google-genai';

const ai = genkit({
  plugins: [googleAI({ apiKey: process.env.GEMINI_API_KEY })],
});

const result = await ai.generate('Tell me a joke');
```

## Plugin Re-exports

Each [plugin](/concepts/plugins.md) defines a function (e.g. `googleAI`, `vertexAI`) re-exported from the plugin package. Plugins register models, tools, retrievers, and other actions into the `Genkit` instance's registry.
