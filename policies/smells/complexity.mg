% Cyclomatic Complexity Smell Detection
% Reads precomputed has_complexity facts and flags functions with high complexity.

query_metadata("smell_high_complexity", "Detect functions with high cyclomatic complexity").
query_metadata("smell_high_complexity", "category", "smell").
query_metadata("smell_high_complexity", "tier", "2").
query_metadata("smell_high_complexity", "severity", "medium").
query_metadata("smell_high_complexity", "smell_type", "high_complexity").
query_metadata("smell_high_complexity", "Predicate", "has_smell_type").

query("smell_high_complexity", File, Sym) :-
    defines(File, Sym),
    triples(Sym, "has_complexity", N),
    N > 15.
