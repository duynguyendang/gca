---
type: Concept
title: Plugins
description: Self-contained packages that register models, tools, retrievers, and other capabilities into the Genkit registry.
tags: [genkit, plugins, extensibility, core-concept]
timestamp: 2026-06-19T00:00:00Z
---

A **Plugin** is a self-contained package that registers [actions](/concepts/actions.md) (models, tools, retrievers, etc.) into the Genkit [registry](/concepts/registry.md).

## Source

- [PluginProvider](core/src/plugin.ts#PluginProvider) — Plugin interface
- [InitializedPlugin](core/src/plugin.ts#InitializedPlugin) — Initialized plugin

## Two Plugin Versions

| Version | Interface | Characteristics |
|---|---|---|
| v1 | `GenkitPlugin` | Original plugin API |
| v2 | `GenkitPluginV2` | Simplified, composable API |

## Plugin Categories

- **Model providers** — google-genai, vertexai, anthropic, compat-oai, ollama
- **Deployment** — express, fastify, fetch, next, firebase
- **Vector stores** — dev-local-vectorstore, chromadb, pinecone, cloud-sql-pg
- **Monitoring** — google-cloud (Cloud Trace, Cloud Monitoring)
- **Safety** — checks (Google Checks AI safety)
- **Evaluation** — evaluators (RAG evaluation)
- **[Middleware](/concepts/middleware.md)** — Retry, fallback, tool approval, etc.
- **[Interop](/plugins/interop-plugins.md)** — mcp (Model Context Protocol), langchain

## Lifecycle

1. User passes plugin factory to `genkit({ plugins: [...] })`
2. Plugin initializes and registers its actions in the [registry](/concepts/registry.md)
3. On shutdown, the `Genkit` instance calls each plugin's cleanup

## Links

- Registered in the [Registry](/concepts/registry.md)
- Full list in [Plugins](/plugins/index.md)
- Defined in [@genkit-ai/core](/core/genkit-core.md)
