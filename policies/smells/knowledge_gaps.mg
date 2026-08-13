% Knowledge Gap Analysis

% Detects isolated symbols with degree 0 (writeDegreeFacts writes numeric objects).
% gap_thin_community (aggregation: clusters with < 3 members) is computed Go-side
% by Analyzer.writeThinCommunities and emits has_knowledge_gap="thin" directly.
query_metadata("gap_isolated", "Detect isolated symbols with degree 0 or 1").
query_metadata("gap_isolated", "category", "knowledge_gap").
query_metadata("gap_isolated", "severity", "high").
query_metadata("gap_isolated", "Predicate", "has_knowledge_gap").

query_metadata("gap_untested_hotspot", "Detect high-degree symbols with no test coverage").
query_metadata("gap_untested_hotspot", "category", "knowledge_gap").
query_metadata("gap_untested_hotspot", "severity", "high").
query_metadata("gap_untested_hotspot", "Predicate", "has_knowledge_gap").

query_metadata("gap_thin_community", "Detect communities with fewer than 3 members").
query_metadata("gap_thin_community", "category", "knowledge_gap").
query_metadata("gap_thin_community", "severity", "medium").
query_metadata("gap_thin_community", "Predicate", "has_knowledge_gap").

query_metadata("gap_single_file_community", "Detect communities where all members live in one file").
query_metadata("gap_single_file_community", "category", "knowledge_gap").
query_metadata("gap_single_file_community", "severity", "low").
query_metadata("gap_single_file_community", "Predicate", "has_knowledge_gap").

query_metadata("gap_isolated_top", "Top-K most isolated symbols").
query_metadata("gap_isolated_top", "category", "knowledge_gap").
query_metadata("gap_isolated_top", "severity", "medium").
query_metadata("gap_isolated_top", "Predicate", "has_knowledge_gap").

query_metadata("gap_untested_top", "Top-K untested hotspots").
query_metadata("gap_untested_top", "category", "knowledge_gap").
query_metadata("gap_untested_top", "severity", "medium").
query_metadata("gap_untested_top", "Predicate", "has_knowledge_gap").

query("gap_isolated", Symbol, "isolated") :-
    triples(Symbol, "has_out_degree", "0"),
    triples(Symbol, "has_in_degree", "0").

query("gap_untested_hotspot", Symbol, "high_degree_untested") :-
    triples(Symbol, "has_out_degree", High),
    High != "0",
    not triples(Symbol, "is_test_symbol", "true").

query("gap_single_file_community", Cluster, File) :-
    triples(Node, "belongs_to_cluster", Cluster),
    triples(Node, "in_file", File).

query("gap_isolated_top", Symbol, "isolated") :-
    triples(Symbol, "has_out_degree", "0"),
    triples(Symbol, "has_in_degree", "0").

query("gap_untested_top", Symbol, "high_degree_untested") :-
    triples(Symbol, "has_out_degree", High),
    High != "0",
    not triples(Symbol, "is_test_symbol", "true").