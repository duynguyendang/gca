---
type: Concept
title: Evaluators
description: AI output quality evaluation — measuring correctness, safety, relevance, and other metrics of model responses.
tags: [genkit, evaluators, evaluation, core-concept]
timestamp: 2026-06-19T00:00:00Z
---

An **Evaluator** is an [Action](/concepts/actions.md) that assesses the quality of AI-generated output.

## Source

- [defineEvaluator](ai/src/evaluator.ts#defineEvaluator) — Define an evaluator
- [evaluator](ai/src/evaluator.ts#evaluator) — Create an evaluator action

## Key Features

- **Custom evaluators** — Define your own scoring logic
- **Built-in evaluators** — Via `@genkit-ai/evaluators` plugin (malicious detection, factual consistency, etc.)
- **Batch evaluation** — Run multiple evaluations across test datasets
- **Typed** — Input/output schemas for evaluation results

## Links

- Packaged in [@genkit-ai/evaluators](/plugins/index.md)
- Defined in [@genkit-ai/ai](/core/genkit-ai.md)
- Built on [@genkit-ai/core](/core/genkit-core.md)
