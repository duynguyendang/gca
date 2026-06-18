% OKF smell: stale concept — okf_timestamp older than 90 days.
% Knowledge becomes stale faster than code. A concept not refreshed in 90 days
% is likely out of date and should be reviewed.
% Weight: 2 (low-medium — informational).

query_metadata("okf_stale_concept", "Detect OKF concepts with stale timestamps").
query_metadata("okf_stale_concept", "category", "smell").
query_metadata("okf_stale_concept", "tier", "2").
query_metadata("okf_stale_concept", "severity", "low").
query_metadata("okf_stale_concept", "smell_type", "okf_stale_concept").
query_metadata("okf_stale_concept", "Predicate", "has_smell_type").

% Conservative threshold: 90 days. We treat the timestamp as a free-form ISO 8601
% string; consumers (frontend, alerts) compute the actual age in Go where date
% math is straightforward. Here we just mark the presence of an old timestamp
% as recorded by the analyzer (which writes okf_age_days alongside the timestamp).
query("okf_stale_concept", Concept, "timestamp_old") :-
    okf_concept(Concept, _),
    okf_age_days(Concept, Days),
    Days > 90.