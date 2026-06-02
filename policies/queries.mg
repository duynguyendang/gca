% GCA Query Registry

Decl triples(S, P, O).
Decl query_metadata(Name, Description).
Decl query_metadata(Name, Key, Value).
Decl query(Name, Arg1, Arg2).

query_metadata("find_defines", "Find all symbols defined in a file").
query_metadata("find_defines", "category", "structural").
query_metadata("find_defines", "tier", "1").

query_metadata("find_imports", "Find all imports for a file").
query_metadata("find_imports", "category", "structural").
query_metadata("find_imports", "tier", "1").

% --- Core Structural Queries ---

find_defines(FileID, Symbol) :-
    triples(FileID, "defines", Symbol).

find_imports(FileID, Target) :-
    triples(FileID, "imports", Target).

% ============================================
% Architecture Smell Detection Rules
% These are query TEMPLATES - they define patterns to find,
% not derived predicates. Execute via the /api/v1/query endpoint
% with the template as the query string.
% ============================================

% --- Circular Dependency Detection ---
query_metadata("smell_circular_direct", "Detect mutual call pairs (A calls B and B calls A)").
query_metadata("smell_circular_direct", "category", "smell").
query_metadata("smell_circular_direct", "tier", "1").
query_metadata("smell_circular_direct", "severity", "high").
query_metadata("smell_circular_direct", "smell_type", "circular_dependency").
query_metadata("smell_circular_direct", "template", "triples(A, \"calls\", B), triples(B, \"calls\", A), A != B").

% --- Files with imports ---
query_metadata("smell_imports", "List files with imports").
query_metadata("smell_imports", "category", "smell").
query_metadata("smell_imports", "tier", "1").
query_metadata("smell_imports", "template", "triples(File, \"imports\", Pkg)").
query_metadata("smell_imports", "description_extended", "Returns all (File, Package) pairs where File imports Package").

% --- Files with defines ---
query_metadata("smell_defines", "List files with definitions").
query_metadata("smell_defines", "category", "smell").
query_metadata("smell_defines", "tier", "1").
query_metadata("smell_defines", "template", "triples(File, \"defines\", Symbol)").
query_metadata("smell_defines", "description_extended", "Returns all (File, Symbol) pairs where File defines Symbol").

% --- Hub Anomaly Detection ---
query_metadata("smell_hub", "Detect hub files (called by and calling many)").
query_metadata("smell_hub", "category", "smell").
query_metadata("smell_hub", "tier", "1").
query_metadata("smell_hub", "severity", "medium").
query_metadata("smell_hub", "smell_type", "hub_anomaly").
query_metadata("smell_hub", "template", "triples(File, \"calls\", _), triples(Caller, \"calls\", File), File != Caller").

% --- Layer Violation Detection ---
query_metadata("smell_layer_violation", "Detect cross-layer dependency violations").
query_metadata("smell_layer_violation", "category", "smell").
query_metadata("smell_layer_violation", "tier", "1").
query_metadata("smell_layer_violation", "severity", "high").
query_metadata("smell_layer_violation", "smell_type", "layer_violation").
query_metadata("smell_layer_violation", "template", "triples(File, \"imports\", Target), triples(File, \"has_tag\", LayerTag), triples(Target, \"has_tag\", \"backend\"), LayerTag != \"backend\"").

% ============================================
% Analytical Store Query Constants
% Used by Go service layer for direct fact lookups.
% ============================================

query_metadata("smell_type", "Look up smell type for a subject").
query_metadata("smell_type", "template", "triples(Subject, \"has_smell_type\", Type)").

query_metadata("smell_severity", "Look up smell severity for a subject").
query_metadata("smell_severity", "template", "triples(Subject, \"has_smell_severity\", Severity)").

query_metadata("smell", "Look up smells for a subject").
query_metadata("smell", "template", "triples(Subject, \"has_smell\", Object)").

query_metadata("hub_score", "Look up hub score for a subject").
query_metadata("hub_score", "template", "triples(Subject, \"has_hub_score\", Score)").

query_metadata("entry_point", "Check if subject is an entry point").
query_metadata("entry_point", "template", "triples(Subject, \"is_entry_point\", \"true\")").

query_metadata("centrality", "Look up centrality score for a subject").
query_metadata("centrality", "template", "triples(Subject, \"has_centrality\", Score)").

query_metadata("in_degree", "Look up in-degree for a subject").
query_metadata("in_degree", "template", "triples(Subject, \"has_in_degree\", Degree)").

query_metadata("out_degree", "Look up out-degree for a subject").
query_metadata("out_degree", "template", "triples(Subject, \"has_out_degree\", Degree)").

query_metadata("cluster", "Look up cluster membership for a subject").
query_metadata("cluster", "template", "triples(Subject, \"belongs_to_cluster\", Cluster)").

query_metadata("health_debt", "Look up health debt for a subject").
query_metadata("health_debt", "template", "triples(Subject, \"has_health_debt\", Debt)").

query_metadata("health_score", "Look up health score for a subject").
query_metadata("health_score", "template", "triples(Subject, \"has_health_score\", Score)").

query_metadata("surprise", "Look up surprise edges for a subject").
query_metadata("surprise", "template", "triples(Subject, \"has_surprise\", Type), triples(Subject, \"calls\", Target)").

query_metadata("surprise_score", "Look up surprise score for a subject").
query_metadata("surprise_score", "template", "triples(Subject, \"has_surprise_score\", ScoreStr)").

query_metadata("in_degree_short", "Look up in-degree for a symbol").
query_metadata("in_degree_short", "template", "triples(S, \"has_in_degree\", D)").

query_metadata("out_degree_short", "Look up out-degree for a symbol").
query_metadata("out_degree_short", "template", "triples(S, \"has_out_degree\", D)").

query_metadata("cluster_short", "Look up cluster for a symbol").
query_metadata("cluster_short", "template", "triples(S, \"belongs_to_cluster\", C)").

query_metadata("test_symbol", "Check if symbol is a test symbol").
query_metadata("test_symbol", "template", "triples(S, \"is_test_symbol\", \"true\")").

query_metadata("in_file", "Look up file containing a symbol").
query_metadata("in_file", "template", "triples(S, \"in_file\", F)").

% ---- Graph traversal query templates ----

query_metadata("defines", "Find symbols defined in a file").
query_metadata("defines", "template", "triples(File, \"defines\", Symbol)").

query_metadata("imports", "Find imports for a file").
query_metadata("imports", "template", "triples(File, \"imports\", Target)").

query_metadata("calls_from", "Find outgoing calls from a node").
query_metadata("calls_from", "template", "triples(Node, \"calls\", Target)").

query_metadata("calls_to", "Find incoming calls to a node").
query_metadata("calls_to", "template", "triples(Caller, \"calls\", Node)").

query_metadata("all_calls", "Return all call edges").
query_metadata("all_calls", "template", "triples(Caller, \"calls\", Callee)").

query_metadata("query_template_body", "Get query template body").
query_metadata("query_template_body", "template", "triples(TemplateID, \"query_template\", Body)").

query_metadata("has_kind", "Query by kind predicate").
query_metadata("has_kind", "template", "triples(Subject, \"has_kind\", Kind)").

query_metadata("has_tag", "Query by tag predicate").
query_metadata("has_tag", "template", "triples(Subject, \"has_tag\", Tag)").

% ---- Cross-reference templates ----

query_metadata("who_calls", "Find who calls a symbol").
query_metadata("who_calls", "template", "triples(Caller, \"calls\", Symbol)").

query_metadata("what_calls", "Find what a symbol calls").
query_metadata("what_calls", "template", "triples(Symbol, \"calls\", Callee)").

query_metadata("smell_weight", "Look up smell weight from policies").
query_metadata("smell_weight", "template", "triples(Name, \"smell_weight\", Weight)").

% ---- Analyzer queries ----

query_metadata("hub_candidates", "Find files that call other symbols (hub detection)").
query_metadata("hub_candidates", "template", "triples(File, \"calls\", _), not contains(File, \":\")").

query_metadata("entry_candidates", "Find files that define main/init (entry point detection)").
query_metadata("entry_candidates", "template", "triples(File, \"defines\", Symbol), or(contains(Symbol, \"main\"), contains(Symbol, \"init\"))").

query_metadata("symbol_calls", "Find call edges through file-defined symbols (centrality)").
query_metadata("symbol_calls", "template", "triples(File, \"defines\", Symbol), triples(Symbol, \"calls\", Target)").
