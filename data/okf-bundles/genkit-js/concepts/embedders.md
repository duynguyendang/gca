---
type: Concept
title: Embedders
description: Text embedding models that convert text into vector representations for semantic search and RAG.
tags: [genkit, embedders, embeddings, core-concept]
timestamp: 2026-06-19T00:00:00Z
---

An **Embedder** is an [Action](/concepts/actions.md) that converts text into a dense vector representation (embedding). Embeddings are used for semantic search and RAG.

## Source

- [defineEmbedder](ai/src/embedder.ts#defineEmbedder) — Define an embedding model
- [embed](ai/src/embedder.ts#embed) — Generate embeddings for text

## Key Features

- **Batch support** — `embed()` handles single texts; `embedMany()` for batches
- **Model-agnostic** — Unified interface across embedding providers
- **Plugged into retrievers** — [Retrievers](/concepts/retrievers.md) use embedders to find relevant documents

## Links

- Used by [Retrievers](/concepts/retrievers.md)
- Defined in [@genkit-ai/ai](/core/genkit-ai.md)
- Built on [@genkit-ai/core](/core/genkit-core.md)
