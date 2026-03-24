package datalog

import (
	"fmt"
	"sort"
)

// QueryOptimizer optimizes Datalog queries for better performance.
type QueryOptimizer struct {
	// statistics can be added here for cost-based optimization
}

// NewQueryOptimizer creates a new query optimizer.
func NewQueryOptimizer() *QueryOptimizer {
	return &QueryOptimizer{}
}

// OptimizeQuery reorders atoms to minimize intermediate results.
// It uses heuristic rules to determine the optimal execution order:
// 1. Atoms with more bound variables (constants) are executed first
// 2. Atoms with selective predicates (like equality constraints) are prioritized
// 3. Variables that appear in multiple atoms are considered "join variables"
func (o *QueryOptimizer) OptimizeQuery(atoms []Atom) []Atom {
	if len(atoms) <= 2 {
		return atoms // No optimization needed for simple queries
	}

	// Calculate priority for each atom
	priorities := make([]int, len(atoms))
	for i, atom := range atoms {
		priorities[i] = o.calculateAtomPriority(atom, atoms)
	}

	// Sort atoms by priority (higher priority first)
	sortedAtoms := make([]Atom, len(atoms))
	copy(sortedAtoms, atoms)

	sort.Slice(sortedAtoms, func(i, j int) bool {
		return priorities[i] > priorities[j]
	})

	return sortedAtoms
}

// calculateAtomPriority calculates the execution priority of an atom.
// Higher priority means the atom should be executed earlier.
// Returns a priority score (higher = better to execute first).
func (o *QueryOptimizer) calculateAtomPriority(atom Atom, atoms []Atom) int {
	score := 0

	// Priority 1: Count bound variables (constants vs variables)
	// More constants = more selective = higher priority
	boundCount := 0
	totalArgs := len(atom.Args)

	for _, arg := range atom.Args {
		if o.isBound(arg) {
			boundCount++
		}
	}

	if totalArgs > 0 {
		// Scale by the ratio of bound variables
		score += (boundCount * 100) / totalArgs
	}

	// Priority 2: Selective predicates get higher priority
	switch atom.Predicate {
	case "neq", "!=", "regex", "contains", "starts_with":
		score += 50 // Constraint predicates are very selective
	case "eq", "=":
		score += 40
	case "type", "kind":
		score += 30 // Type predicates are usually selective
	case "has_role", "has_tag", "has_doc":
		score += 20 // Role predicates have medium selectivity
	case "triples":
		score += 10 // Triples is the base predicate, lowest priority among these
	default:
		// Unknown predicate, give low priority
		score += 5
	}

	// Priority 3: Atoms with variables that appear elsewhere (join variables)
	// These are important for connecting results
	for _, arg := range atom.Args {
		arg = trimQuotes(arg)
		if isVariable(arg) {
			// This is a variable that needs to be joined
			score += 5 // Small bonus for join variables
		}
	}

	return score
}

// isBound determines if an argument is bound (constant) or unbound (variable).
// Variables start with '?' (e.g., ?x, ?s, ?o)
func (o *QueryOptimizer) isBound(arg string) bool {
	// Remove quotes if present
	arg = trimQuotes(arg)
	// Variables start with '?'
	return !isVariable(arg)
}

// isVariable checks if an argument is a variable.
func isVariable(arg string) bool {
	arg = trimQuotes(arg)
	return len(arg) > 0 && arg[0] == '?'
}

// trimQuotes removes surrounding quotes from an argument.
func trimQuotes(arg string) string {
	if len(arg) >= 2 {
		if (arg[0] == '"' && arg[len(arg)-1] == '"') ||
			(arg[0] == '\'' && arg[len(arg)-1] == '\'') {
			return arg[1 : len(arg)-1]
		}
	}
	return arg
}

// AnalyzeVariables analyzes a query to find variable dependencies.
// Returns a map of variables to the atoms that use them.
func (o *QueryOptimizer) AnalyzeVariables(atoms []Atom) map[string][]int {
	variables := make(map[string][]int)

	for i, atom := range atoms {
		for _, arg := range atom.Args {
			arg = trimQuotes(arg)
			if isVariable(arg) {
				variables[arg] = append(variables[arg], i)
			}
		}
	}

	return variables
}

// EstimateCost estimates the cost of executing a set of atoms.
// Lower cost is better. This is a simple heuristic cost model.
func (o *QueryOptimizer) EstimateCost(atoms []Atom) int {
	cost := 0

	// Base cost for each atom
	cost += len(atoms) * 10

	// Add cost for unbound variables (potential joins)
	variables := o.AnalyzeVariables(atoms)
	for _, atomIndices := range variables {
		if len(atomIndices) > 1 {
			// Join cost increases with the number of atoms to join
			cost += len(atomIndices) * 5
		}
	}

	return cost
}

// PredicatePushdown pushes selective predicates into the scan operation.
// This optimization moves constraints into the store scan to reduce data transfer.
func (o *QueryOptimizer) PredicatePushdown(atoms []Atom) ([]Atom, map[string]string) {
	pushdownPredicates := make(map[string]string)
	optimizedAtoms := make([]Atom, 0, len(atoms))

	for _, atom := range atoms {
		if atom.Predicate == "neq" || atom.Predicate == "!=" {
			// Push inequality constraints
			if len(atom.Args) == 2 {
				varName := trimQuotes(atom.Args[0])
				if isVariable(varName) {
					pushdownPredicates[varName] = fmt.Sprintf("neq:%s", trimQuotes(atom.Args[1]))
					// Don't add to optimized atoms; it's pushed down
					continue
				}
			}
		} else if atom.Predicate == "eq" || atom.Predicate == "=" {
			// Push equality constraints
			if len(atom.Args) == 2 {
				varName := trimQuotes(atom.Args[0])
				value := trimQuotes(atom.Args[1])
				if isVariable(varName) && !isVariable(value) {
					pushdownPredicates[varName] = value
					continue
				}
			}
		}

		optimizedAtoms = append(optimizedAtoms, atom)
	}

	return optimizedAtoms, pushdownPredicates
}

// CreateExecutionPlan creates an optimized execution plan for a query.
// The plan specifies the order of atom execution and any pushed-down predicates.
type ExecutionPlan struct {
	Atoms         []Atom            // Ordered atoms to execute
	PushdownPreds map[string]string // Predicates pushed to scan
	EstimatedCost int               // Estimated execution cost
}

// CreateExecutionPlan creates an optimized execution plan for the query.
func (o *QueryOptimizer) CreateExecutionPlan(atoms []Atom) *ExecutionPlan {
	// Step 1: Apply predicate pushdown
	optimizedAtoms, pushdownPreds := o.PredicatePushdown(atoms)

	// Step 2: Reorder atoms for optimal join order
	orderedAtoms := o.OptimizeQuery(optimizedAtoms)

	// Step 3: Estimate cost
	cost := o.EstimateCost(orderedAtoms)

	return &ExecutionPlan{
		Atoms:         orderedAtoms,
		PushdownPreds: pushdownPreds,
		EstimatedCost: cost,
	}
}
