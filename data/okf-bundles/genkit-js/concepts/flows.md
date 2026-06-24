---
type: Concept
title: Flows
description: Observable, streamable, strongly-typed workflow functions. Can be served as HTTP endpoints.
tags: [genkit, flows, workflows, core-concept]
timestamp: 2026-06-19T00:00:00Z
---

A **Flow** is a strongly-typed, observable, streamable [Action](/concepts/actions.md) that serves as the primary way to define multi-step AI processes in Genkit.

## Source

- [defineFlow](core/src/flow.ts#defineFlow) — Define a typed flow
- [Flow](core/src/flow.ts#Flow) — Flow interface

## Key Features

- **Strongly typed** — Input and output schemas are validated at runtime via Zod
- **Observable** — Full OpenTelemetry tracing of every step
- **Streamable** — Flows can stream intermediate results
- **Served as HTTP** — Exposed via Express, Fastify, Fetch, or Next.js [plugins](/plugins/index.md)
- **Client callable** — Remote flows can be invoked via `runFlow()` from the `genkit` client package

## Example

```typescript
import { genkit, z } from 'genkit';

const ai = genkit({});

export const jokeFlow = ai.defineFlow(
  {
    name: 'jokeFlow',
    inputSchema: z.string(),
    outputSchema: z.string(),
  },
  async (topic) => {
    const { text } = await ai.generate(`Tell me a joke about ${topic}`);
    return text;
  }
);
```

## Links

- Used with [Generate](/concepts/generate.md)
- Calls [Tools](/concepts/tools.md) during execution
- Configured via [Middleware](/concepts/middleware.md)
- Provides session context for [Chat](/concepts/chat-sessions.md)
- Defined in [@genkit-ai/core](/core/genkit-core.md)
- Served via [Deployment Plugins](/plugins/deployment-plugins.md)
- Traced via [Telemetry](/telemetry/tracing.md)
