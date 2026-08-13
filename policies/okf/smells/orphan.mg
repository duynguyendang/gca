% OKF smell: orphan concept — has no inbound okf_link and no bridges_to.
% These are knowledge artifacts that nothing else in the bundle references;
% they're candidates for either deletion or re-linking.
% Weight: 3 (medium).
%
% Engine note: derived predicates (okf_concept, okf_link, bridges_to) are NOT
% stored as facts, so rule bodies here use their triples forms directly.
% `not triples(...)` is store-aware existential negation (supported).

query_metadata("okf_orphan_concept", "Detect OKF concepts with no inbound links or bridges").
query_metadata("okf_orphan_concept", "category", "smell").
query_metadata("okf_orphan_concept", "tier", "1").
query_metadata("okf_orphan_concept", "severity", "medium").
query_metadata("okf_orphan_concept", "smell_type", "okf_orphan_concept").
query_metadata("okf_orphan_concept", "Predicate", "has_smell_type").

query("okf_orphan_concept", Concept, "no_inbound") :-
    triples(Concept, "okf_concept", _),
    not triples(_, "okf_link", Concept),
    not triples(Concept, "bridges_to", _).