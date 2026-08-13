% Composite Severity Scoring — Health Debt Calculation

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
smell_weight("unsanitized_db_access", 50).
smell_weight("security_risk", 50).

hub_high(File) :- triples(File, "has_hub_score", "high").

file_has_smell(File, SmellType) :- triples(File, "has_smell", SmellType).

file_smell_weight(File, Weight) :-
  file_has_smell(File, SmellType),
  smell_weight(SmellType, Weight).

file_smell_weight(File, 0) :-
  file_has_smell(File, SmellType),
  not smell_weight(SmellType, _).

health_debt_with_hub(File, Total) :-
  file_smell_weight(File, Weight),
  Weight > 0,
  Total #= Weight + (hub_high(File) ? 5 : 0).

health_debt_with_hub(File, Total) :-
  file_smell_weight(File, Weight),
  Weight > 0,
  not hub_high(File),
  Total #= Weight.

health_score(File, Score) :-
  health_debt_with_hub(File, Debt),
  Score #= 100 - Debt.

health_score(File, 100) :-
  not file_has_smell(File, _),
  not hub_high(File).