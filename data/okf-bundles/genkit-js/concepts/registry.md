---
type: Concept
title: Registry
description: Central catalog that holds all actions, plugins, and schemas in a Genkit instance. Supports 16 action types.
tags: [genkit, registry, catalog, core-concept]
timestamp: 2026-06-19T00:00:00Z
---

The **Registry** is the central catalog that stores and resolves all actions, plugins, and schemas within a `Genkit` instance.

## Source

- [Registry](core/src/registry.ts#Registry) — Central registry class

## Responsibilities

- Store **actions** by type and name (16 action types: model, tool, flow, retriever, embedder, evaluator, reranker, etc.)
- Store **plugins** and their providers
- Store **schemas** for runtime validation
- Resolve action references by name (e.g. `'gemini-2.5-flash'` → the model action)
- Support action listing (used by the [Reflection API](/concepts/reflection-api.md))

## Usage

The registry is internal to each `Genkit` instance. Users don't interact with it directly — it's populated by [plugins](/concepts/plugins.md) and queried by [actions](/concepts/actions.md) and the [Reflection API](/concepts/reflection-api.md).

## Links

- Populated by [Plugins](/concepts/plugins.md)
- Exposed via [Reflection API](/concepts/reflection-api.md)
- Defined in [@genkit-ai/core](/core/genkit-core.md)
