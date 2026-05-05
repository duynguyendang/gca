% Circular Dependency Detection
% Detects mutual call pairs (A calls B and B calls A)

query_metadata("smell_circular_direct", "Detect mutual call pairs (A calls B and B calls A)").
query_metadata("smell_circular_direct", "category", "smell").
query_metadata("smell_circular_direct", "tier", "1").
query_metadata("smell_circular_direct", "severity", "high").
query_metadata("smell_circular_direct", "smell_type", "circular_dependency").

query_metadata("smell_circular_direct", "Predicate", "has_smell_type").
query_metadata("smell_circular_direct", "template", `triples(A, "calls", B), triples(B, "calls", A), A != B`).

query("smell_circular_direct", A, B) :-
    triples(A, "calls", B),
    triples(B, "calls", A),
    A != B.

% Transitive Cycle Detection (3-step)
% Detects A->B->C->A cycles where A, B, C are distinct files
query_metadata("smell_circular_transitive", "Detect 3-step transitive cycles (A imports B, B imports C, C imports A)").
query_metadata("smell_circular_transitive", "category", "architecture_smell").
query_metadata("smell_circular_transitive", "tier", "2").
query_metadata("smell_circular_transitive", "severity", "high").
query_metadata("smell_circular_transitive", "smell_type", "circular_transitive").
query_metadata("smell_circular_transitive", "description", "Detects files involved in 3-step import cycles (A→B→C→A), which indicate structural coupling and risk of cascading changes.").
query_metadata("smell_circular_transitive", "remediation", "Refactor one of the import paths to break the cycle; consider extracting a shared interface or moving shared code to a lower-level module.").
query_metadata("smell_circular_transitive", "Predicate", "has_smell_type").
query_metadata("smell_circular_transitive", "template", `triples(A, "imports", B), triples(B, "imports", C), triples(C, "imports", A), A != B, B != C, A != C`).

query("smell_circular_transitive", A, C) :-
    triples(A, "imports", B),
    triples(B, "imports", C),
    triples(C, "imports", A),
    A != B,
    B != C,
    A != C.

query("smell_circular_transitive", A, D) :-
    triples(A, "imports", B),
    triples(B, "imports", C),
    triples(C, "imports", D),
    triples(D, "imports", A),
    A != B,
    B != C,
    C != D,
    A != C,
    A != D,
    B != D.
