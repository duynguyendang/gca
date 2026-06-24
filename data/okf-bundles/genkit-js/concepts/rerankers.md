---
type: Concept
title: Rerankers
description: Result relevance re-ranking — improve the quality of retrieved documents by re-ordering them by relevance.
tags: [genkit, rerankers, reranking, core-concept]
timestamp: 2026-06-19T00:00:00Z
---

A **Reranker** is an [Action](/concepts/actions.md) that re-orders retrieved documents by relevance to improve RAG result quality.

## Source

- [defineReranker](ai/src/reranker.ts#defineReranker) — Define a reranker
- [reranker](ai/src/reranker.ts#reranker) — Create a reranker action

## Purpose

After a [retriever](/concepts/retrievers.md) fetches candidate documents, a reranker applies a more sophisticated relevance model to re-order them, keeping only the most relevant results.

## Key Features

- **Two-stage retrieval** — Retrievers fetch broadly; rerankers refine
- **Plugged into the generation pipeline** — Automatically applied during RAG
- **Model-agnostic** — Unified interface across reranker providers

## Links

- Used with [Retrievers](/concepts/retrievers.md)
- Used during [Generate](/concepts/generate.md)
- Defined in [@genkit-ai/ai](/core/genkit-ai.md)
