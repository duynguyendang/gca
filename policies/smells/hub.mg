% Hub Anomaly Detection
% Detects files that are called by many (in-degree) and call many (out-degree)

query_metadata("smell_hub", "Detect hub files with high out-degree and in-degree").
query_metadata("smell_hub", "category", "smell").
query_metadata("smell_hub", "tier", "1").
query_metadata("smell_hub", "severity", "medium").
query_metadata("smell_hub", "smell_type", "hub_anomaly").
query_metadata("smell_hub", "Predicate", "has_smell_type").
query_metadata("smell_hub", "template", `triples(File, "calls", _), triples(_, "calls", File), File != _`).

query("smell_hub", File, "high_connectivity") :-
    triples(File, "has_out_degree", Out),
    triples(File, "has_in_degree", In),
    triples(File, "is_hub", "true").