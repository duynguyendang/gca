% Security Smell Detection

query_metadata("smell_unsanitized_db_access", "Detect paths from public APIs to databases without sanitizers").
query_metadata("smell_hardcoded_secret", "Detect hardcoded passwords, API keys, tokens, or secrets").
query_metadata("smell_insecure_crypto", "Detect use of weak cryptographic algorithms (MD5, SHA1, DES, RC4)").
query_metadata("smell_missing_error_check", "Detect DB operations without error handling").

direct_call(A, B) :- triples(A, "calls", B).

% Unsanitized DB access: public API calls database directly without sanitizer
query("smell_unsanitized_db_access", API, DB) :-
    triples(API, "has_tag", "public_api"),
    triples(DB, "has_tag", "database"),
    direct_call(API, DB).

% Hardcoded secrets: files containing password, api_key, token, secret, credential
query("smell_hardcoded_secret", File, Name) :-
    triples(File, "defines", Sym),
    regex(Sym, "password|api[_-]?key|token|secret|credential|private[_-]?key"),
    Name = "hardcoded_secret".

% Insecure crypto: use of MD5, SHA1, DES, RC4,ECB mode
query("smell_insecure_crypto", File, Algo) :-
    triples(File, "references", Ref),
    regex(Ref, "md5|sha1|des|rc4|ecb|paddington").

% Missing error check: call to DB/routine without error handling pattern
query("smell_missing_error_check", File, Call) :-
    triples(File, "calls", Callee),
    regex(Callee, "query|exec|transaction|commit|rollback"),
    not(triples(File, "calls", ErrCheck)),
    ErrCheck = "error".

query_metadata("security_hub", "Detect public API files that call into other components (potential security hubs)").
query_metadata("security_hub", "category", "security").
query_metadata("security_hub", "severity", "medium").
query_metadata("security_hub", "Predicate", "has_security_smell").

query("security_hub", File) :-
    triples(File, "has_tag", "public_api"),
    triples(File, "calls", Callee).