% Health scoring weights.
%
% The smell_weight/2 facts below are the SINGLE SOURCE OF TRUTH for health
% scoring weights. They are read by:
%   • pkg/common.LoadSmellWeights → analyzer_scoring.go:computeHealthScores
%     (Go-side pass that writes has_health_debt / has_health_score).
%   • pkg/registry/smell_registry.go (SmellRegistry policy fallback).
%
% The old Datalog rules below used derived predicates (file_has_smell,
% smell_weight, hub_high, health_debt_with_hub) plus `not <derived>` atoms.
% mebpkg.Query is a triple-store evaluator: derived predicates are NOT stored as
% facts, so those rule bodies would error loudly (docs/designs/contract.md §5).
% The Go scorer implements the same semantics — see analyzer_scoring.go.

query_metadata("scoring_health_debt", "category", "scoring").
query_metadata("scoring_health_debt", "severity", "high").
query_metadata("scoring_health_debt", "predicate", "has_health_debt").

query_metadata("scoring_health_score", "category", "scoring").
query_metadata("scoring_health_score", "severity", "medium").
query_metadata("scoring_health_score", "predicate", "has_health_score").

smell_weight("circular_dependency", 10).
smell_weight("circular_transitive", 15).
smell_weight("layer_violation", 8).
smell_weight("god_file", 6).
smell_weight("hub_anomaly", 4).
smell_weight("dead_code", 3).
smell_weight("high_complexity", 8).
smell_weight("duplicate_code", 4).
smell_weight("unsanitized_db_access", 50).
smell_weight("security_risk", 50).
smell_weight("hardcoded_secret", 50).
smell_weight("insecure_crypto", 40).
smell_weight("missing_error_check", 30).
smell_weight("vulnerable_dependency", 50).