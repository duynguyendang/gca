---
type: Reference
title: Safety, Evaluation & Monitoring Plugins
description: AI safety classification and RAG evaluation plugins.
tags: [genkit, plugins, safety, evaluation]
timestamp: 2026-06-19T00:00:00Z
---

| Plugin | Package | Purpose |
|---|---|---|
| Google Checks | `@genkit-ai/checks` | Google Checks AI safety classification |
| Evaluators | `@genkit-ai/evaluators` | Built-in RAG evaluation (malicious detection, factual consistency, etc.) |
| Middleware | `@genkit-ai/middleware` | Retry, fallback, tool approval, filesystem, skills middleware |

## Monitoring

| Plugin | Package | Purpose |
|---|---|---|
| Google Cloud | `@genkit-ai/google-cloud` | GCP monitoring, Cloud Trace, Cloud Monitoring, telemetry export |

## Links

- [Evaluators concept](/concepts/evaluators.md) — Evaluators are [Actions](/concepts/actions.md)
- [Middleware concept](/concepts/middleware.md) — wraps [Actions](/concepts/actions.md)
- [Tracing](/telemetry/tracing.md)
