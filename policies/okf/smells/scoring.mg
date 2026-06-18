% OKF smell weights.
% Loaded via policies/init.mg -> load_policy("okf/smells/scoring.mg").

smell_weight("okf_orphan_concept", 3).
smell_weight("okf_stale_concept", 2).
smell_weight("okf_hub_anomaly", 4).
smell_weight("okf_bridge_break", 5).