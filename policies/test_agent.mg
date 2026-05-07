% Test Agent Policy — Datalog rules for test generation context
% Provides transitive dependency resolution for mocking context building.
% Loaded via init.mg → load_policy("test_agent.mg").

query_metadata("get_test_dependencies", "category", "testing").
query_metadata("get_test_dependencies", "description", "Get all transitive dependencies for mocking context").

% Base case: direct outgoing call
test_dep(A, B) :- triples(A, "calls", B).

% Recursive case: transitive calls (Mangle handles cycles safely via bottom-up eval)
test_dep(A, C) :- triples(A, "calls", B), test_dep(B, C).

query_template("get_test_dependencies", "template",
    "test_dep({Target}, Dep)."
).