# 🏆 GCA Master Test Suite: Full-Stack Architectural Intelligence

**Objective**: To verify the complete reasoning chain of GCA—from raw ingestion to high-level AI insights. The Agent must fix any underlying logic (Ingestor, Pathfinder, or AI Prompting) until all cases pass.

---

## 🏗️ Phase 1: Ingestion & Structural Integrity (REL)

*Goal: Verify that the triple store correctly maps the project's static structure.*

| ID | Test Mission | Tool | Success Criteria |
| --- | --- | --- | --- |
| **REL-01** | **API Bridge**: Link FE functions to BE handlers. | `query_datalog` | Found triples: `?f --(calls_api)--> ?u` and `?u --(handled_by)--> ?h`. |
| **REL-02** | **Role Tagging**: Identify API handlers & utilities. | `query_datalog` | All symbols in `pkg/server` have `role: "api_handler"`. |
| **REL-03** | **Dead Code**: Find uncalled FE utilities. | `query_datalog` | Returns symbols in `gca-fe/utils` with **In-degree = 0**. |
| **REL-04** | **Package Integrity**: Project prefixing. | `query_datalog` | 100% of symbols start with `gca-be/` or `gca-fe/`. |

---

## 🌉 Phase 2: Cross-Stack Connectivity (BFS)

*Goal: Verify the "Pathfinder" can bridge the FE-BE gap with sub-200ms latency.*

| ID | Test Mission | Source  Target | Success Criteria |
| --- | --- | --- | --- |
| **BFS-01** | **E2E Execution Trace**: UI to Engine. | `App.tsx`  `executor.go` | Path found via **API Portal**. Result is a **Skeleton** (nodes < 15). |
| **BFS-02** | **Data Propagation**: Model to UI. | `ast.go:Term`  `geminiService.ts` | Traces how a BE struct change affects FE parsing logic. |
| **BFS-03** | **Weighted Search**: Logic over Folders. | `utils.ts`  `main.go` | Prefers `calls` (Weight 1) over `in_package` (Weight 10). |
| **BFS-04** | **Safety Limits**: Branching Factor. | Any High-degree Node | BFS expansion capped at **50 nodes/level**. No system hang. |

---

## 🧠 Phase 3: Advanced AI Reasoning (AI)

*Goal: Verify Gemini's ability to act as an "Architectural Expert" using Backend context.*

| ID | Category | User Query / Scenario | Expected AI Action |
| --- | --- | --- | --- |
| **AI-01** | **Semantic Insight** | "What is the purpose of `graphService.ts`?" | Summarizes architectural role using BE source + docs. |
| **AI-02** | **Impact Analysis** | "If I rename `Fact.ID` to `UUID`, what breaks in FE?" | Traces propagation from Go struct to React UI components. |
| **AI-03** | **Error Trace** | "How does the UI handle a 500 error from the BFS API?" | Explains flow from `pathfinder.go` (error) to FE `ErrorBanner`. |
| **AI-04** | **Audit** | "Are any components calling APIs directly without a service?" | Scans for `fetch` calls in `.tsx` that bypass `graphService.ts`. |
| **AI-05** | **What-if** | "I want to add 'Export PDF'. Where should I start?" | Provides a roadmap: `Service` -> `Hook` -> `Component`. |

---

## ⚡ Phase 4: Performance & Quality Standards

*Goal: Ensure the system is "Production-Ready" on Cloud Run.*

1. **Latency**:
* Datalog/BFS queries: **< 200ms**.
* Full AI Reasoning (including tool calls): **< 3s**.


2. **Accuracy**:
* Zero "Hallucinations": AI must cite the specific file/line found in BadgerDB.
* No Node Bloat: Visualization must always be "Skeleton-only."


3. **Stability**:
* Handles sharded ingestion without memory leaks.
* Truncates overly long strings (>64KB) to protect BadgerDB.



---

## 📋 Agent Action Plan: "The Grand Validation"

> **Agent, follow these steps to certify the system:**
> 1. **Data Sync**: Perform a clean ingestion. Ensure prefixes `gca-be/` and `gca-fe/` are applied.
> 2. **Execution**: Run all tests from **REL-01 to AI-05** sequentially.
> 3. **Validation**:
> * For **REL/BFS**: Verify against raw Datalog results.
> * For **AI**: Validate the "Reasoning Chain"—did it call the right tools?
> 
> 
> 4. **Self-Heal**: If a trace fails (e.g., INT-01 returns 0 nodes), identify the missing bridge triple and fix the Go/TS Extractor.
> 5. **Final Report**: Generate a table with `ID | Status | Latency | Key Findings`.
> 
> 