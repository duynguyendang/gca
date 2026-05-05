% Knowledge Gap Analysis

query_metadata("gap_isolated", "Detect isolated symbols with degree 0 or 1").
query_metadata("gap_untested_hotspot", "Detect high-degree symbols with no test coverage").
query_metadata("gap_thin_community", "Detect communities with fewer than 3 members").
query_metadata("gap_single_file_community", "Detect communities where all members live in one file").
query_metadata("gap_isolated_top", "Top-K most isolated symbols").
query_metadata("gap_untested_top", "Top-K untested hotspots").

query("gap_isolated", Symbol, "isolated") :-
    triples(Symbol, "has_out_degree", "zero"),
    triples(Symbol, "has_in_degree", "zero").

query("gap_untested_hotspot", Symbol, "high_degree_untested") :-
    triples(Symbol, "has_degree", "high"),
    triples(Symbol, "is_test_symbol", IsTest),
    IsTest != "true".

query("gap_single_file_community", Cluster, File) :-
    triples(Node, "belongs_to_cluster", Cluster),
    triples(Node, "in_file", File).