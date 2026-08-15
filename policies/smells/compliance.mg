% Compliance Smell Detection (F4)
%
% Flags files that import packages with high-severity known vulnerabilities from
% the offline advisory snapshot. The compliance matcher writes these stored
% facts to the Analytical Store before templates run:
%   has_vulnerability(Package, AdvisoryID)
%   vuln_severity(AdvisoryID, Severity)
%   vuln_summary(AdvisoryID, Summary)
% The rule below inlines the derived 3-arg predicate from the design into the
% stored triple form (docs/designs/contract.md §5 — derived predicates error).

query_metadata("smell_vulnerable_dependency", "Detect imports of packages with high-severity known vulnerabilities").
query_metadata("smell_vulnerable_dependency", "category", "compliance").
query_metadata("smell_vulnerable_dependency", "tier", "2").
query_metadata("smell_vulnerable_dependency", "severity", "high").
query_metadata("smell_vulnerable_dependency", "smell_type", "vulnerable_dependency").
query_metadata("smell_vulnerable_dependency", "Predicate", "has_smell_type").

% Files importing a package with a high-severity advisory.
query("smell_vulnerable_dependency", File, Package) :-
    triples(File, "imports", Package),
    triples(Package, "has_vulnerability", AdvisoryID),
    triples(AdvisoryID, "vuln_severity", Severity),
    eq(Severity, "high").