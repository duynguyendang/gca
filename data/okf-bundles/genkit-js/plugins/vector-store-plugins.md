---
type: Reference
title: Vector Store Plugins
description: Vector database plugins for RAG — storing and querying embeddings for semantic search.
tags: [genkit, plugins, vector-stores, rag, chromadb, pinecone]
timestamp: 2026-06-19T00:00:00Z
---

| Plugin | Package | Purpose |
|---|---|---|
| Dev Local Vector Store | `@genkit-ai/dev-local-vectorstore` | In-memory vector store for development |
| ChromaDB | `genkitx-chromadb` | Open-source vector database |
| Pinecone | `genkitx-pinecone` | Managed vector database |
| Cloud SQL pgvector | `genkitx-cloud-sql-pg` | PostgreSQL vector store on GCP Cloud SQL |

## Links

- [Retrievers concept](/concepts/retrievers.md) — how vector stores are queried (Retrievers are [Actions](/concepts/actions.md))
- [Embedders concept](/concepts/embedders.md) — how text becomes vectors (Embedders are [Actions](/concepts/actions.md))
