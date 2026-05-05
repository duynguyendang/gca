% Layer Violation Detection
% Detects cross-layer dependency violations

query_metadata("smell_layer_violation", "Detect cross-layer dependency violations").

query("smell_layer_violation", File, Target) :-
    triples(File, "imports", Target),
    triples(Target, "has_tag", "backend"),
    triples(File, "has_tag", LayerTag),
    LayerTag != "backend".