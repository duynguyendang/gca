% Surprise Scoring — Detect unexpected architectural coupling

query_metadata("surprise_cross_community", "Detect calls crossing community boundaries").
query_metadata("surprise_cross_language", "Detect calls between different language files").
query_metadata("surprise_peripheral_hub", "Detect low-degree nodes calling high-degree hubs").
query_metadata("surprise_cross_test_boundary", "Detect production code calling test code").
query_metadata("surprise_score", "Composite surprise score for call edges").
query_metadata("surprise_top", "Top-K most surprising call edges").
query_metadata("surprise_hotspot", "Files with most surprising coupling").

query("surprise_cross_community", Source, Target) :-
    triples(Source, "calls", Target),
    triples(Source, "belongs_to_cluster", SrcCluster),
    triples(Target, "belongs_to_cluster", TgtCluster),
    SrcCluster != TgtCluster.

query("surprise_cross_language", Source, Target) :-
    triples(Source, "calls", Target),
    triples(Source, "has_language", SrcLang),
    triples(Target, "has_language", TgtLang),
    SrcLang != TgtLang.

query("surprise_cross_test_boundary", Source, Target) :-
    triples(Source, "calls", Target),
    triples(Source, "is_test_symbol", TestMark),
    TestMark != "true",
    triples(Target, "is_test_symbol", "true").