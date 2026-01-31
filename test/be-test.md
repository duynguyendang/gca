# 🚀 GCA Integration Test Protocol (Backend AI Architect)

**Role**: Senior QA Engineer / Architecture Validator.
**Objective**: Execute 10 critical test cases to verify the integrity of the Knowledge Graph, the accuracy of cross-stack pathfinding, and the reasoning quality of Gemini-on-Backend.

---

## 🛠️ Phase 1: Ingestion & Environment Setup

Before running tests, the Agent must:

1. **Clean State**: Reset the BadgerDB FactStore.
2. **Ingestion**: Run the full ingestion for `gca-be/` and `gca-fe/` in source-code/gca folder.
3. **Verification**: Ensure all symbols use the standardized project prefixes.

---

## 🧪 Phase 2: The Integration Test Suite

### 1. Cross-Stack Connectivity (The Bridge)

| Test ID | User Query / Mission | Tool to Use | Success Criteria |
| --- | --- | --- | --- |
| **INT-01** | "Trace the flow from the search input in `App.tsx` to the query engine in Go." | `find_connection` | A path exists: `App.tsx` -> `useSmartSearch` -> `graphService` -> `/v1/graph/query` -> `handlers.go` -> `executor.go`. |
| **INT-02** | "Find which Go handlers are triggered by `useGraphData.ts`." | `query_datalog` | Returns a list of Go functions linked via `calls_api` and `handled_by`. |
| **INT-03** | "Verify the API link for the endpoint `/v1/graph/path`." | `query_datalog` | Confirms exactly 1 URI node connecting FE `fetchGraphPath` to BE `handleGraphPath`. |

### 🧠 2. AI Architectural Reasoning (Gemini-on-BE)

| Test ID | User Query / Mission | Agent Strategy | Success Criteria |
| --- | --- | --- | --- |
| **AI-01** | "What is the purpose of `graphService.ts` in the frontend?" | Gemini reads source + docs via BE. | Returns an insight: "Abstraction layer for graph API calls with caching logic." |
| **AI-02** | "If I modify the `Term` struct in `ast.go`, which FE services need updates?" | **Impact Analysis**: Trace `Term` -> Handler -> URL -> FE Service. | Identifies all `.ts` files that parse the `Term` structure from API responses. |
| **AI-03** | "Explain the error handling flow from Go Executor to the React UI." | BFS search for Error nodes. | Traces the propagation of errors from Go's `Result` type to React's Error Boundary. |

### ⚡ 3. Performance & BFS Quality

| Test ID | User Query / Mission | Tool to Use | Success Criteria |
| --- | --- | --- | --- |
| **PERF-01** | **Skeleton Check**: Run INT-01 and count nodes. | `find_connection` | **Result `nodeCount` < 15**. No noise/neighbor nodes allowed in the response. |
| **PERF-02** | **Latency Check**: Measure cross-stack trace time. | Timer on BFS API. | **Execution time < 200ms** on the local environment. |
| **PERF-03** | **Weighted Search**: Trace from `utils.ts` to `main.go`. | `find_connection` | Path must prefer `calls` (logic) over `in_package` (structure) even if the path is longer. |

### 🧹 4. Code Health (Relational Checks)

| Test ID | User Query / Mission | Tool to Use | Success Criteria |
| --- | --- | --- | --- |
| **CL-01** | "Find frontend utilities with zero incoming calls." | `query_datalog` | Identifies symbols in `gca-fe/utils` with `in-degree = 0`. |
| **CL-02** | "List all API handlers that do not have a documentation string." | `query_datalog` | Returns functions with `role: api_handler` missing the `has_doc` fact. |

---

## 📈 Phase 3: Reporting & Self-Healing

After execution, the Agent must provide a report in the following format:

### **Test Results Summary**

| ID | Status | Latency | Reasoning Logic / Path Taken |
| --- | --- | --- | --- |
| INT-01 | **PASS** | 142ms | App.tsx -> useSmartSearch -> graphService -> handlers.go |
| AI-02 | **FAIL** | N/A | Missing `exposes_model` triple in Go ingestor. |

**Self-Healing Protocol**:

* If any **INT** or **AI** test fails due to missing links, the Agent must **inspect the Ingestor code**, fix the triple extraction logic (e.g., improve string literal detection for APIs), re-ingest, and re-test until **PASS**.

---