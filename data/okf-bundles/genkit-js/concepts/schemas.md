---
type: Concept
title: Schemas
description: Runtime schema validation using Zod with JSON Schema interoperability. Used everywhere in Genkit — models, tools, flows, prompts.
tags: [genkit, schemas, zod, validation, core-concept]
timestamp: 2026-06-19T00:00:00Z
---

**Schemas** in Genkit provide runtime type validation and JSON Schema generation via [Zod](https://zod.dev).

## Source

- [defineSchema](core/src/schema.ts#defineSchema) — Define a named schema
- [toJsonSchema](core/src/schema.ts#toJsonSchema) — Convert Zod to JSON Schema
- [annotateSchema](core/src/schema.ts#annotateSchema) — Add metadata to a schema

## Usage

Schemas are used throughout Genkit to validate [action](/concepts/actions.md) inputs and outputs:
- [Flow](/concepts/flows.md) input/output types
- [Tool](/concepts/tools.md) input/output types
- [Model](/concepts/models.md) structured output
- [Prompt](/concepts/prompts.md) input validation
- [Generate](/concepts/generate.md) output formatting

## Key Features

- **Zod schemas** — Define with `z.object({...})`, `z.string()`, etc.
- **JSON Schema export** — `toJsonSchema()` converts Zod schemas to JSON Schema for model consumption
- **Schema annotations** — `annotateSchema()` adds metadata to schema fields
- **Define schema** — `defineSchema()` creates a named, reusable schema in the registry

## Example

```typescript
import { z } from 'genkit';

const JokeSchema = z.object({
  setup: z.string(),
  punchline: z.string(),
  rating: z.number().min(1).max(10),
});
```

## Links

- Stored in the [Registry](/concepts/registry.md)
- Used to define [Chat](/concepts/chat-sessions.md) session state
- Defined in [@genkit-ai/core](/core/genkit-core.md)
- Re-exported from [genkit](/core/genkit.md)
