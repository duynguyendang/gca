% OKF (Open Knowledge Format v0.1) — predicate declarations
% Loaded via policies/init.mg -> load_policy("okf/_decl.mg").
% All predicates must match pkg/okf/predicates.go constant names exactly.
% Mismatches fail at policy-load time (the registry validates declarations).

% --- Source Store predicates ---

.decl okf_concept(Concept: string, Type: string)
.decl okf_title(Concept: string, Title: string)
.decl okf_description(Concept: string, Description: string)
.decl okf_resource(Concept: string, URI: string)
.decl okf_tag(Concept: string, Tag: string)
.decl okf_timestamp(Concept: string, Timestamp: string)
.decl okf_body(Concept: string, Body: string)
.decl okf_link(Concept: string, Target: string)
.decl okf_content_hash(Concept: string, Hash: string)
.decl okf_frontmatter(Concept: string, JSON: string)
.decl okf_version(Project: string, Version: string)
% okf_event deferred — see design §okf_event-deferred

% --- Analytical Store predicates ---

.decl bridges_to(Concept: string, Symbol: string)
.decl okf_bridge_miss(Concept: string, TargetURI: string)