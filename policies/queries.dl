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
