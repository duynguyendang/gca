% Security Smell Detection
%
% Engine note: this file's rule bodies run through executeRulesFromTemplates →
% mebpkg.Query. See docs/designs/contract.md §5 (Engine Capability Matrix).
% regex()/contains()/not(triples(...)) are supported; derived predicates and
% `not <derived>` are NOT stored as facts and error loudly.
%
% Template rules here: smell_hardcoded_secret, smell_insecure_crypto (both regex
% over source-store triples).
%
% Go-side passes (analyzer_security.go), because they need graph joins that the
% row-filter model cannot express (file→file reachability through symbol-level
% `calls` facts, or negation over call graphs):
%   • smell_unsanitized_db_access — public-api file directly reaches a database
%     file via a symbol-level call chain.
%   • smell_missing_error_check — file calls a DB-ish symbol with no error
%     handling call anywhere in the file.

query_metadata("smell_unsanitized_db_access", "Detect paths from public APIs to databases without sanitizers").
query_metadata("smell_hardcoded_secret", "Detect hardcoded passwords, API keys, tokens, or secrets").
query_metadata("smell_insecure_crypto", "Detect use of weak cryptographic algorithms (MD5, SHA1, DES, RC4)").
query_metadata("smell_missing_error_check", "Detect DB operations without error handling").

% Exact has_smell_type values that must be classified as security smells.
% Read by common.LoadSecuritySmellTypes (SmellRegistry fallback).
security_type("unsanitized_db_access").
security_type("hardcoded_secret").
security_type("insecure_crypto").
security_type("missing_error_check").

% Template smells: regex over source-store triples.
query_metadata("smell_hardcoded_secret", "category", "security").
query_metadata("smell_hardcoded_secret", "severity", "high").
query_metadata("smell_hardcoded_secret", "smell_type", "hardcoded_secret").
query_metadata("smell_hardcoded_secret", "Predicate", "has_smell_type").

% Hardcoded secrets: files defining symbols whose name suggests a password/key/token.
% The regex matches on the symbol name (source-store defines triples).
query("smell_hardcoded_secret", File, Sym) :-
    triples(File, "defines", Sym),
    regex(Sym, "password|api[_-]?key|token|secret|credential|private[_-]?key").

query_metadata("smell_insecure_crypto", "category", "security").
query_metadata("smell_insecure_crypto", "severity", "high").
query_metadata("smell_insecure_crypto", "smell_type", "insecure_crypto").
query_metadata("smell_insecure_crypto", "Predicate", "has_smell_type").

% Insecure crypto: files referencing weak algorithm symbols.
query("smell_insecure_crypto", File, Ref) :-
    triples(File, "references", Ref),
    regex(Ref, "md5|sha1|des|rc4|ecb|padding").

% Removed templates (implemented Go-side in analyzer_security.go):
%   - smell_unsanitized_db_access: previously direct_call(API, DB); file-level
%     `calls` triples do not exist, so this can never match as a template.
%   - smell_missing_error_check: the old body not(triples(File,"calls",ErrCheck))
%     meant "File has NO outgoing calls" and always returned zero rows.
%   - security_hub: same file-level `calls` limitation.
