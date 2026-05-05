% Attention Sink Promotion Rules
% Facts matching these rules are stored in the Global (Attention Sink) partition

query_metadata("check_sticky", "description", "Check if a fact should be sticky (Attention Sink)").
query_metadata("check_sticky", "template", "is_sticky('{Predicate}', '{Subject}')").

is_sticky("has_security_risk", Subject) :- triples(Subject, "has_security_risk", _).
is_sticky("defines", Symbol) :- triples(Symbol, "has_role", "api_gateway").
is_sticky("defines", Symbol) :- triples(Symbol, "has_role", "authenticator").
is_sticky("handles", Symbol) :- triples(Symbol, "has_permission", "admin").

is_sticky("backbone", File) :- triples(File, "has_complexity", "high").

is_sticky("defines", Symbol) :- triples(Symbol, "has_role", "main").
is_sticky("defines", Symbol) :- triples(Symbol, "has_role", "init").