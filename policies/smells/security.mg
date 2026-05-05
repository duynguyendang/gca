% Security Smell Detection

query_metadata("smell_unsanitized_db_access", "Detect paths from public APIs to databases without sanitizers").

direct_call(A, B) :- triples(A, "calls", B).

query("smell_unsanitized_db_access", API, DB) :-
    triples(API, "has_tag", "public_api"),
    triples(DB, "has_tag", "database"),
    direct_call(API, DB).

security_hub(File) :-
    triples(File, "has_tag", "public_api"),
    triples(File, "calls", Callee).