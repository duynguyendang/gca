# 🧪 Mangle Library Test Suite

**Objective**: Verify the architectural integrity of the `google/mangle` Datalog library. Since this is a pure library (no UI), tests focus on package inter-dependencies and logical correctness.

---

## 🏗️ Phase 1: Structural Integrity (REL)

*Goal: Ensure the library structure respects Go modularity.*

| ID | Test Mission | Tool | Success Criteria |
| --- | --- | --- | --- |
| **REL-01** | **Core Analysis**: Find `analysis.Analyze`. | `query_datalog` | Found triples: `?s --(name)--> "Analyze"`, `?s --(in_package)--> "mangle/analysis"`. |
| **REL-02** | **AST Concepts**: Verify `Constant` type exists. | `query_datalog` | Found `mangle/ast/ast.go:Constant` struct. |
| **REL-03** | **Package Isolation**: No cyclic deps. | `query_datalog` | `ast` package should NOT import `engine`. |

---

## 🌉 Phase 2: Traceability (BFS)

*Goal: Verify code navigation logic works for Go libraries.*

| ID | Test Mission | Source -> Target | Success Criteria |
| --- | --- | --- | --- |
| **BFS-01** | **Parser to AST**: Trace usage. | `parse/parse.go` -> `ast/ast.go` | Path found. `parse` function returns `ast` types. |
| **BFS-02** | **Engine to Unions**: Trace Core Logic. | `engine/engine.go` -> `unionfind/unionfind.go` | Path found. Engine uses Union-Find for resolution. |
| **BFS-03** | **Limit Check**: Large Fan-out. | `ast/ast.go:Term` | BFS stays within 50 nodes. |

---

## 🧠 Phase 3: AI Understanding (AI)

*Goal: Verify Gemini can explain complex Datalog algorithms.*

| ID | Category | User Query | Expected AI Action |
| --- | --- | --- | --- |
| **AI-01** | **Explanation** | "How does the `Unify` function work?" | Summarizes unification algorithm in `engine`. |
| **AI-02** | **Impact** | "If I change `ast.Constant` to an interface, what breaks?" | Identifies `parse` and `engine` usage. |

---

## ⚡ Phase 4: Performance

1. **Latency**: Datalog queries < 200ms.
