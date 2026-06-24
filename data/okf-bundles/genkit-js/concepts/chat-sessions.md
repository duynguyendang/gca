---
type: Concept
title: Chat & Sessions
description: Multi-turn conversation state management — chat sessions with history, context, and session persistence.
tags: [genkit, chat, sessions, conversation, core-concept]
timestamp: 2026-06-19T12:00:00Z
---

**Chat** and **Sessions** provide stateful multi-turn conversation management.

## Source

- [Chat](ai/src/chat.ts#Chat) — Chat session management
- [Session](ai/src/session.ts#Session) — Session state management
- [SessionStore](ai/src/session.ts#SessionStore) — Persistence interface

## Key Features

- **Stateful conversations** — `Chat.send()` calls the [generate](/concepts/generate.md) pipeline and appends history automatically; the model sees full context
- **Streaming** — `Chat.sendStream()` yields response [chunks](/concepts/generate.md) as they arrive
- **Multiple threads** — One session can host several named chat conversations, each with its own [context execution](/concepts/flows.md) scope
- **Custom session state** — Store user context, auth data, or any app state alongside chat history, optionally validated with [Schemas](/concepts/schemas.md)
- **Pluggable persistence** — `SessionStore` interface with in-memory default; swap in any backend via [Plugins](/concepts/plugins.md)
- **Preamble support** — Inject system prompts or [pre-rendered prompt templates](/concepts/prompts.md) at the start of each conversation

## Chat

Chat objects manage conversation history and provide a [`send()`](/concepts/generate.md) method that calls the generation pipeline and automatically appends each exchange to the message list:

```typescript
const chat = ai.chat({ model: 'gemini-2.5-flash' });
const r1 = await chat.send('Hi, my name is Alice');
const r2 = await chat.send('What is my name?'); // remembers "Alice"
```

### Streaming

[`sendStream()`](/concepts/generate.md) returns a `GenerateStreamResponse` with a `stream` channel:

```typescript
const { stream, response } = chat.sendStream('Tell me a long story');
for await (const chunk of stream) {
  process.stdout.write(chunk.text);
}
const final = await response;
```

### History

Access the current message list at any point:

```typescript
chat.messages; // MessageData[] — full conversation so far
```

## Sessions

Sessions provide persistent state across multiple chat turns:

- **Conversation history** — preserved under named threads
- **Custom state** — `session.state` / `session.updateState()` for user context, auth tokens, etc.
- **Multiple threads** — separate conversations within one session
- **Restorable** — sessions can be serialized and rehydrated

### State

```typescript
const session = ai.createSession({ initialState: { userId: 'abc' } });
session.state; // { userId: 'abc' }
await session.updateState({ theme: 'dark' });
session.state; // { userId: 'abc', theme: 'dark' }
```

### Threads

```typescript
const session = ai.createSession({});
const support = session.chat('support', { system: 'You are a support agent' });
const billing = session.chat('billing', { system: 'You handle billing' });
```

### Context execution

[`session.run()`](/concepts/flows.md) sets up a scoped execution context for the session:

```typescript
session.run(() => {
  const current = ai.currentSession();
  // work within session context
});
```

## Session Store

Sessions use a `SessionStore` to persist data. The default is an in-memory store; custom stores backed by Redis, SQLite, or any database implement the interface (typically provided via [Plugins](/concepts/plugins.md)):

```typescript
interface SessionStore<S = any> {
  get(sessionId: string): Promise<SessionData<S> | undefined>;
  save(sessionId: string, data: Omit<SessionData<S>, 'id'>): Promise<void>;
}
```

## Links

- Runs on a [Model](/concepts/models.md) [Action](/concepts/actions.md)
- Calls [Tools](/concepts/tools.md) during generation
- Configured via [Middleware](/concepts/middleware.md)
- Can use [Prompts](/concepts/prompts.md) as preambles
- Validated with [Schemas](/concepts/schemas.md)
- Used inside [Flows](/concepts/flows.md)
- Implemented in [@genkit-ai/ai](/core/genkit-ai.md)
- Built on [@genkit-ai/core](/core/genkit-core.md)
