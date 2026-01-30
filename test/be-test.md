# 🚀 MASTER TASK: Full-Stack Reasoning & BFS Pathfinding Validation

**Objective**: Verify the Backend's ability to find multi-hop paths across Frontend and Backend using the `find_connection` tool. The Agent must fix the Pathfinder (BFS) and Ingestor until all paths are accurately discovered and "skeletonized."

---

## 🏗️ Phase 1: The "Connectivity" Ingestion Rules

The Agent must ensure these triples exist to allow the BFS to "jump" layers:

* **Semantic Portals**: Link API calls to handlers via URI nodes.
* **Logical Ownership**: Link files to their primary exports.
* **Weighted Edges**: Assign priorities so BFS prefers `calls` over `in_package`.

---

## 🧪 Phase 2: `find_connection` Tool Test Cases (BFS Validation)

The Agent must run the `find_connection` tool for these specific pairs and verify the output.

### 1. The "Inception" Trace (FE  BE Logic)

* **Source**: `gca-fe/App.tsx`
* **Target**: `gca-be/pkg/service/pathfinder.go`
* **Requirement**: BFS must find a path that crosses the API Bridge (e.g., via `/v1/graph/path`).
* **Success**: Returns a **Skeleton Path** (exactly the nodes on the path, no noise).

### 2. Data Propagation Trace (Model  UI)

* **Source**: `gca-be/pkg/ast/ast.go:Term` (or equivalent data struct)
* **Target**: `gca-fe/services/geminiService.ts`
* **Requirement**: BFS must navigate from a Backend Model through a Handler, across the API, into a Frontend Service.
* **Success**: Prove that a change in the Go struct has a logical path to the TypeScript parsing logic.

### 3. Deep Utility Discovery (Component  Helper)

* **Source**: `gca-fe/components/TreeVisualizer.tsx`
* **Target**: `gca-fe/utils/pathfinding.ts:bfsPath`
* **Requirement**: Verify internal Frontend connectivity.
* **Success**: The path must show exactly which Hook or Service bridges the UI to the Utility.

---

## 🔍 Phase 3: Datalog Logic Test Cases (Relational Validation)

In addition to BFS, run these to verify "Global Knowledge":

* **Case A (Dead Code)**: Find `role: "utility"` nodes with **In-degree = 0**.
* **Case B (API Inventory)**: List all `role: "api_handler"` and their `exposes_model` targets.
* **Case C (Package Integrity)**: Ensure all symbols have an `in_package` triple.

---

## 📈 Phase 4: The "Show-off" Performance Standards

The Agent is not done until the `find_connection` tool meets these quality bars:

1. **Skeleton Pruning**: The result JSON must contain **ONLY** the nodes and links in the path. `nodeCount` must be **< 15** for any trace.
2. **Branching Control**:
* Limit expansion to **50 nodes per level**.
* If BFS reaches **Depth 10** without a target, it must terminate gracefully.
* **Math Constraint**: Ensure the search space  stays within memory limits.


3. **Path Labeling**: Edges in the path must have semantic types: `calls_api`, `handled_by`, `calls`.

---

## 📋 Acceptance Checklist for the Agent

* [ ] **Cross-Stack**: Can `find_connection` jump from `.ts` to `.go` files?
* [ ] **Precision**: Does the tool return a clean "Skeleton" or a "Cloud" of nodes? (Skeleton is mandatory).
* [ ] **Speed**: Do all 3 BFS test cases return in **< 500ms**?
* [ ] **Heuristics**: If a symbol-to-symbol path fails, does the tool automatically try a **File-to-File** path?

---