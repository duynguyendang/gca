% Composite Severity Scoring — Health Debt Calculation

query_metadata("scoring_health_debt", "Calculate health debt per file").

smell_weight("circular_dependency", 10).
smell_weight("circular_transitive", 15).
smell_weight("layer_violation", 8).
smell_weight("god_file", 6).
smell_weight("hub_anomaly", 4).
smell_weight("unsanitized_db_access", 50).

hub_high(File) :- triples(File, "has_hub_score", "high").

file_has_smell(File, SmellType) :- triples(File, "has_smell", SmellType).