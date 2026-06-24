---
type: Concept
title: Tools
description: Functions that models can call automatically (function calling). Supports interrupts and human-in-the-loop.
tags: [genkit, tools, function-calling, core-concept]
timestamp: 2026-06-19T00:00:00Z
---

A **Tool** is an [Action](/concepts/actions.md) that a model can invoke during [generation](/concepts/generate.md). Tools enable LLMs to access external data, perform computations, or trigger side effects.

## Source

- [defineTool](ai/src/tool.ts#defineTool) — Define a callable tool
- [defineInterrupt](ai/src/tool.ts#defineInterrupt) — Define an interrupt
- [interrupt](ai/src/tool.ts#interrupt) — Pause execution for human input
- [restartTool](ai/src/tool.ts#restartTool) — Resume after an interrupt

## Key Features

- **Automatic resolution** — When a model requests a tool, Genkit automatically calls it and feeds the result back
- **Interrupts** — Human-in-the-loop: tools can pause execution and wait for user input
- **Typed** — Input and output schemas via Zod
- **Observable** — Tool invocations are traced

## Example

```typescript
import { genkit, z } from 'genkit';

const ai = genkit({});

const weatherTool = ai.defineTool(
  {
    name: 'getWeather',
    description: 'Get the current weather for a location',
    inputSchema: z.object({ location: z.string() }),
    outputSchema: z.string(),
  },
  async ({ location }) => {
    return `The weather in ${location} is sunny and 72°F.`;
  }
);
```

## Links

- Used during [Generate](/concepts/generate.md)
- Registered in the [Registry](/concepts/registry.md)
- Can be wrapped with [Middleware](/concepts/middleware.md)
- Called during [Chat](/concepts/chat-sessions.md) generation
- Defined in [@genkit-ai/ai](/core/genkit-ai.md)
