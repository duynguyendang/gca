---
type: Concept
title: Prompts
description: Prompt-as-code using Dotprompt format — Markdown frontmatter with Handlebars templating and Zod schemas.
tags: [genkit, prompts, dotprompt, core-concept]
timestamp: 2026-06-19T00:00:00Z
---

Prompts in Genkit are defined as code using **Dotprompt** format — a Markdown file with YAML frontmatter and Handlebars template variables.

## Source

- [definePrompt](ai/src/prompt.ts#definePrompt) — Define a Dotprompt prompt
- [loadPromptFolder](ai/src/prompt.ts#loadPromptFolder) — Load prompts from a directory

## Dotprompt Format

```dotprompt
---
model: gemini-2.5-flash
input:
  schema:
    topic: string
    style?: string
  default:
    style: funny
---

You are a comedian. Tell a {{style}} joke about {{topic}}.
```

## Key Features

- **Frontmatter** — Model selection, input schema, config, tools
- **Handlebars** — Template variables, conditionals, helpers
- **Validation** — Input schemas via Zod
- **Folder loading** — `loadPromptFolder()` loads all `.prompt` files from a directory
- **Reusable** — `definePartial()` for shared prompt fragments

## Links

- Consumed by [Generate](/concepts/generate.md) [Actions](/concepts/actions.md)
- Selects a [Model](/concepts/models.md)
- Validated with [Schemas](/concepts/schemas.md)
- Used as preamble in [Chat](/concepts/chat-sessions.md)
- Defined in [@genkit-ai/ai](/core/genkit-ai.md)
- Built on [@genkit-ai/core](/core/genkit-core.md)
