---
type: Concept
title: Retrievers
description: Document retrieval for RAG (Retrieval Augmented Generation). Retrievers fetch relevant documents from a knowledge base.
tags: [genkit, retrievers, rag, core-concept]
timestamp: 2026-06-19T00:00:00Z
---

A **Retriever** is an [Action](/concepts/actions.md) that fetches relevant documents from a knowledge base for use in RAG (Retrieval Augmented Generation).

## Source

- [defineRetriever](ai/src/retriever.ts#defineRetriever) — Define a retriever
- [retriever](ai/src/retriever.ts#retriever) — Create a retriever action
- [retrieve](ai/src/retriever.ts#retrieve) — Run a retrieval query

## Key Features

- **Typed** — Input query and output documents are schema-validated
- **Retrievers wrap vector stores** — They query [embedders](/concepts/embedders.md) to find semantically similar content
- **Simple retriever** — `defineSimpleRetriever()` for quick definitions from existing data

## Links

- Used by [Generate](/concepts/generate.md) for RAG
- Backed by [Vector Store Plugins](/plugins/vector-store-plugins.md)
- Uses [Embedders](/concepts/embedders.md) for semantic search
- Defined in [@genkit-ai/ai](/core/genkit-ai.md)
