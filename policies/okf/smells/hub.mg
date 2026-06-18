% OKF smell: hub anomaly — concept with high okf_link fan-in.
% Analogue of policies/smells/hub.mg but scoped to OKF-only edges.
% Note: okf_hub_anomaly does NOT use is_hub because that fact is only written
% by the code centrality pass (writeDegreeFacts only writes is_hub for code files,
% not OKF concepts). It reads has_in_degree which IS written for OKF subjects by
% the extended writeDegreeFacts.
% Weight: 4 (medium-high — high fan-in on a knowledge concept means it's a
% critical cross-reference point).

query_metadata("okf_hub_anomaly", "Detect OKF concepts with high inbound link fan-in").
query_metadata("okf_hub_anomaly", "category", "smell").
query_metadata("okf_hub_anomaly", "tier", "1").
query_metadata("okf_hub_anomaly", "severity", "medium").
query_metadata("okf_hub_anomaly", "smell_type", "okf_hub_anomaly").
query_metadata("okf_hub_anomaly", "Predicate", "has_smell_type").

% Threshold 5: five inbound links is enough to flag; the upstream UI can re-rank.
query("okf_hub_anomaly", Concept, "high_fan_in") :-
    okf_concept(Concept, _),
    has_in_degree(Concept, FanIn),
    FanIn >= 5.