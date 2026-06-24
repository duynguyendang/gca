---
type: Concept
title: Reflection API
description: Development-only API server (port 3100) that exposes the registry for the Genkit Developer UI to inspect and trigger actions.
tags: [genkit, reflection-api, developer-ui, dev-server]
timestamp: 2026-06-19T00:00:00Z
---

The **Reflection API** is a development-only HTTP server that exposes the [registry](/concepts/registry.md) and [actions](/concepts/actions.md) for the Genkit Developer UI.

## Source

- [ReflectionServer](core/src/reflection.ts#ReflectionServer) — V1 reflection server
- [ReflectionServerV2](core/src/reflection-v2.ts#ReflectionServerV2) — V2 reflection server

## Features

- Lists all registered actions, flows, models, tools, etc.
- Invokes actions on demand (for testing in the Developer UI)
- Provides action metadata (schemas, descriptions, config)
- OpenAPI spec at `core/src/api/reflectionApi.yaml`

## Protocol

- Express-based server on port 3100 (development only)
- Not included in production builds
- Not started automatically — only when the Developer UI connects

## Links

- Reads from the [Registry](/concepts/registry.md)
- Defined in [@genkit-ai/core](/core/genkit-core.md)
- Used by the Genkit Developer UI (separate tool)
