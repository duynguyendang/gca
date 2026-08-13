% OKF bridges — read-only consumer of bridges_to facts.
% The Go ingestor (pkg/okf) is the SOLE writer of bridges_to.
% This file contains smell rules that flag stale or suspicious bridges.
% Engine note: bridges_to/defines are stored as triples; the rule body uses their
% triples forms (derived predicates are not stored facts under mebpkg.Query).

% --- Bridge-break smell ---
% A bridges_to fact pointing to a symbol that no longer exists in the Source Store.
% Caught after a refactor removes/renames the symbol. Weight: 5 (critical —
% a bridge that no longer resolves breaks the OKF → code cross-reference contract).

query_metadata("okf_bridge_break", "Detect bridges_to that point to deleted/renamed symbols").
query_metadata("okf_bridge_break", "category", "smell").
query_metadata("okf_bridge_break", "tier", "1").
query_metadata("okf_bridge_break", "severity", "high").
query_metadata("okf_bridge_break", "smell_type", "okf_bridge_break").
query_metadata("okf_bridge_break", "Predicate", "has_smell_type").

query("okf_bridge_break", Concept, Symbol) :-
    triples(Concept, "bridges_to", Symbol),
    not triples(_, "defines", Symbol).

% --- Visible bridge predicate ---
% Mark bridges worth showing in the UI: symbol must exist and have non-trivial centrality.

.decl okf_bridge_visible(Concept: string, Symbol: string)

okf_bridge_visible(Concept, Symbol) :-
    bridges_to(Concept, Symbol),
    has_kind(Symbol, Kind),
    Kind != "file",
    has_centrality(Symbol, Score),
    Score > 0.01.