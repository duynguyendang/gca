---
type: Concept
title: Middleware
description: Composable functions that wrap the generation pipeline. Built-in implementations for retry, fallback, tool approval, filesystem, and skills.
tags: [genkit, middleware, retry, fallback, core-concept]
timestamp: 2026-06-19T00:00:00Z
---

**Middleware** wraps [actions](/concepts/actions.md) and the [generation](/concepts/generate.md) pipeline with composable cross-cutting behavior.

## Source

- [GenerateMiddleware](ai/src/generate/middleware.ts#GenerateMiddleware) — Middleware interface
- [MiddlewareDesc](ai/src/generate/middleware.ts#MiddlewareDesc) — Middleware descriptor type

## Purpose

Middleware can:
- Modify the request before it reaches the model
- Intercept and transform the response
- Handle errors with retry/fallback logic
- Augment the context with additional data

## Built-in Middleware (`@genkit-ai/middleware`)

| Middleware | Purpose |
|---|---|
| `retry` | Retry on failure with configurable backoff |
| `fallback` | Fall back to alternative models on failure |
| `toolApproval` | Require human approval before tool execution |
| `filesystem` | Provide tools for file system access |
| `skills` | Pre-built reusable capabilities |

## Links

- Applied during [Generate](/concepts/generate.md)
- Applied to [Chat](/concepts/chat-sessions.md) sessions
- Packaged in [@genkit-ai/middleware](/plugins/index.md)
- Defined in [@genkit-ai/ai](/core/genkit-ai.md)
- Built on [@genkit-ai/core](/core/genkit-core.md)
