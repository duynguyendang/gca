% GCA Policy Seed Manifest
% Single source of truth — edit this file to configure which policies load.
% Missing or malformed init.mg causes fatal error at startup.
% Files are loaded in declaration order.

load_policy("queries.mg").

load_policy("smells/_decl.mg").
load_policy("smells/circular.mg").
load_policy("smells/god_file.mg").
load_policy("smells/hub.mg").
load_policy("smells/layer.mg").
load_policy("smells/security.mg").
load_policy("smells/surprise.mg").
load_policy("smells/knowledge_gaps.mg").
load_policy("smells/complexity.mg").
load_policy("smells/scoring.mg").

load_policy("okf/_decl.mg").
load_policy("okf/bridges.mg").
load_policy("okf/smells/orphan.mg").
load_policy("okf/smells/stale.mg").
load_policy("okf/smells/hub.mg").
load_policy("okf/smells/scoring.mg").

load_policy("security_agent.mg").
load_policy("quality_agent.mg").
load_policy("performance_agent.mg").
load_policy("logic_consistency_agent.mg").
load_policy("impact_agent.mg").

load_policy("test_agent.mg").

load_policy("intent_templates.mg").
load_policy("memory/promotion.mg").