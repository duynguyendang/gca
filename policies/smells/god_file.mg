% God File / God Module Detection
% Detects files with excessive imports or definitions

query_metadata("smell_god_file", "Detect files with excessive imports or definitions").
query_metadata("smell_god_file", "category", "smell").
query_metadata("smell_god_file", "tier", "1").
query_metadata("smell_god_file", "severity", "medium").
query_metadata("smell_god_file", "smell_type", "god_file").
query_metadata("smell_god_file", "Predicate", "has_smell_type").

query("smell_god_file", File, "excessive_imports") :-
    triples(File, "has_import_count", "excessive").

query("smell_god_file", File, "excessive_defines") :-
    triples(File, "has_define_count", "excessive").