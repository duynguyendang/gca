% Surprise Scoring — Detect unexpected architectural coupling

query_metadata("surprise_cross_community", "Detect calls crossing community boundaries").
query_metadata("surprise_cross_community", "category", "surprise").
query_metadata("surprise_cross_community", "severity", "high").
query_metadata("surprise_cross_community", "Predicate", "has_surprise").

query_metadata("surprise_cross_language", "Detect calls between different language files").
query_metadata("surprise_cross_language", "category", "surprise").
query_metadata("surprise_cross_language", "severity", "medium").
query_metadata("surprise_cross_language", "Predicate", "has_surprise").

query_metadata("surprise_peripheral_hub", "Detect low-degree nodes calling high-degree hubs").
query_metadata("surprise_peripheral_hub", "category", "surprise").
query_metadata("surprise_peripheral_hub", "severity", "medium").
query_metadata("surprise_peripheral_hub", "Predicate", "has_surprise").

query_metadata("surprise_cross_test_boundary", "Detect production code calling test code").
query_metadata("surprise_cross_test_boundary", "category", "surprise").
query_metadata("surprise_cross_test_boundary", "severity", "high").
query_metadata("surprise_cross_test_boundary", "Predicate", "has_surprise").

query_metadata("surprise_score", "Composite surprise score for call edges").
query_metadata("surprise_score", "category", "surprise").
query_metadata("surprise_score", "severity", "low").
query_metadata("surprise_score", "Predicate", "has_surprise_score").

query_metadata("surprise_top", "Top-K most surprising call edges").
query_metadata("surprise_top", "category", "surprise").
query_metadata("surprise_top", "severity", "low").
query_metadata("surprise_top", "Predicate", "has_surprise").

query_metadata("surprise_hotspot", "Files with most surprising coupling").
query_metadata("surprise_hotspot", "category", "surprise").
query_metadata("surprise_hotspot", "severity", "medium").
query_metadata("surprise_hotspot", "Predicate", "has_surprise").

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

query("surprise_peripheral_hub", Source, Target) :-
    triples(Source, "calls", Target),
    triples(Source, "has_out_degree", "zero"),
    triples(Target, "has_out_degree", High),
    High != "zero".

query("surprise_score", Source, Target, Score) :-
    triples(Source, "calls", Target),
    triples(Source, "belongs_to_cluster", SrcCluster),
    triples(Target, "belongs_to_cluster", TgtCluster),
    Score = 1,
    SrcCluster != TgtCluster.

query("surprise_top", Source, Target, Score) :-
    triples(Source, "calls", Target),
    triples(Source, "belongs_to_cluster", SrcCluster),
    triples(Target, "belongs_to_cluster", TgtCluster),
    SrcCluster != TgtCluster,
    Score = 1.

query("surprise_hotspot", File, Count) :-
    triples(Source, "calls", Target),
    triples(Source, "in_file", File),
    triples(Source, "belongs_to_cluster", SrcCluster),
    triples(Target, "belongs_to_cluster", TgtCluster),
    SrcCluster != TgtCluster,
    Count > 0.