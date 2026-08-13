% Hub Anomaly Detection
% Detects files that are called by many (in-degree) and call many (out-degree)

query_metadata("smell_hub", "Detect hub files with high out-degree and in-degree").
query_metadata("smell_hub", "category", "smell").
query_metadata("smell_hub", "tier", "1").
query_metadata("smell_hub", "severity", "medium").
query_metadata("smell_hub", "smell_type", "hub_anomaly").
query_metadata("smell_hub", "Predicate", "has_smell_type").
query_metadata("smell_hub", "template", `triples(File, "calls", _), triples(_, "calls", File), File != _`).

% Engine note: is_hub is NOT a stored fact. computeCentrality writes
% has_hub_score (numeric caller count) to both stores only when the count
% exceeds config.HubClassificationThreshold, so its presence means "hub".
query("smell_hub", File, "high_connectivity") :-
    triples(File, "has_hub_score", _).