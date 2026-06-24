---
type: Concept
title: Models
description: LLM model definitions and model references. Genkit provides a unified interface for 20+ model providers.
tags: [genkit, models, llm, core-concept]
timestamp: 2026-06-19T00:00:00Z
---

A **Model** in Genkit is an [Action](/concepts/actions.md) that wraps an LLM provider's API into a unified interface.

## Source

- [defineModel](ai/src/model.ts#defineModel) — Define a model
- [modelRef](ai/src/model.ts#modelRef) — Create a named model reference

`ai/src/model-types.ts` — Core model types and schemas.

## Key Features

- **Unified interface** — Same API for Gemini, Claude, GPT, Ollama, and more
- **Model references** — `modelRef()` creates a named reference that the [registry](/concepts/registry.md) resolves
- **Configurable** — Temperature, top-p, max tokens, stop sequences, safety settings
- **Streaming** — Models support streaming responses natively
- **Tool calling** — Models that support function calling get tools passed automatically

## Supported Model Providers

| Plugin | Providers |
|---|---|
| `@genkit-ai/google-genai` | Gemini 2.5 Flash, Gemini 2.5 Pro, Gemini 1.5 series |
| `@genkit-ai/vertexai` | Gemini, Imagen, Model Garden, Anthropic on Vertex, Mistral on GCP |
| `@genkit-ai/anthropic` | Claude 3.5 Sonnet, Claude 3 Opus, Claude 3 Haiku |
| `@genkit-ai/compat-oai` | OpenAI GPT-4o, DeepSeek, xAI Grok, any OpenAI-compatible API |
| `genkitx-ollama` | Local models via Ollama (Llama, Mistral, etc.) |

## Links

- Used by [Generate](/concepts/generate.md)
- Selected in [Prompts](/concepts/prompts.md)
- Registered via [Plugins](/concepts/plugins.md)
- Listed in [Model Plugins](/plugins/model-plugins.md)
- Used by [Chat](/concepts/chat-sessions.md) sessions
