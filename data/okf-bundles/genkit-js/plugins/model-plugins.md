---
type: Reference
title: Model Plugins
description: LLM provider plugins that register models into Genkit. Supports Gemini, Claude, GPT, Ollama, and more.
tags: [genkit, plugins, models, llm]
timestamp: 2026-06-19T00:00:00Z
---

| Plugin | Package | Providers |
|---|---|---|
| Google AI | `@genkit-ai/google-genai` | Gemini 2.5 Flash, Gemini 2.5 Pro, Gemini 1.5 series |
| Vertex AI | `@genkit-ai/vertexai` | Gemini, Imagen, Model Garden, Anthropic on Vertex, Mistral on GCP |
| Anthropic | `@genkit-ai/anthropic` | Claude 3.5 Sonnet, Claude 3 Opus, Claude 3 Haiku |
| OpenAI Compatible | `@genkit-ai/compat-oai` | OpenAI GPT-4o/GPT-4o-mini, DeepSeek, xAI Grok |
| Ollama | `genkitx-ollama` | Local models (Llama, Mistral, Gemma, etc.) |

## Links

- [Models concept](/concepts/models.md) — how models are defined and used
- [Plugin concept](/concepts/plugins.md) — how plugins register [actions](/concepts/actions.md)
