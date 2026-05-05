% Intent-to-Datalog Template Mappings
% These templates map user intent to Datalog queries.
% Used by pkg/service/ai/intent.go:GetDatalogTemplateForIntent()

% Who calls: Find callers of a specific symbol or all callers
intent_template("who_calls", "target_known", `triples(?caller, "calls", "{{target}}")`).
intent_template("who_calls", "target_unknown", `triples(?caller, "calls", ?callee)`).

% What calls: Find what a specific symbol calls
intent_template("what_calls", "target_known", `triples("{{target}}", "calls", ?callee)`).
intent_template("what_calls", "target_unknown", `triples(?caller, "calls", ?callee)`).

% How reaches: Path finding between symbols
intent_template("how_reaches", "default", `{"tool": "find_path", "source": "?source", "target": "?target"}`).

% Summarize: Find symbols with documentation in a file
intent_template("summarize", "default", `triples("?target", "defines", ?sym), triples("?target", "has_doc", ?doc)`).

% Explain: Find all facts about a symbol
intent_template("explain", "default", `triples("?target", "?pred", ?obj)`).

% Find: Search for symbols matching a pattern
intent_template("find", "default", `triples(?s, "defines", ?sym), regex(?sym, "?target")`).

% Security: Find references to sensitive data
intent_template("security", "default", `triples(?s, "references", ?ref), regex(?ref, "password|token|secret|key")`).

% Refactor: Find symbols with documentation for refactoring analysis
intent_template("refactor", "default", `triples(?f, "defines", ?sym), triples(?sym, "has_doc", ?doc)`).

% Test generation: Find definitions for test generation
intent_template("test_gen", "default", `triples(?f, "defines", ?sym)`).

% Performance: Find definitions for performance analysis
intent_template("performance", "default", `triples(?f, "defines", ?sym)`).

% Default fallback
intent_template("default", "default", `triples(?s, ?p, ?o)`).
