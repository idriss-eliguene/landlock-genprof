package semantic

import (
	"fmt"
)

// leastFixedPoint computes the least fixed point of phi over the graph's
// current evaluation universe. It returns the least Labelling (grounded)
// or an error if evaluation readiness fails or an internal invariant is
// violated. Package-private; no mutation of Graph.
func (g *Graph) leastFixedPoint() (Labelling, error) {
	univ := g.evaluationUniverse()
	// finite-universe precondition satisfied by in-memory representation
	// validate readiness for the entire universe
	if err := g.validateEvaluationUniverse(); err != nil {
		return Labelling{}, err
	}
	// start from bottom
	current := BottomLabelling()
	// iteration bound: at most |E| + 1 phi applications
	max := len(univ) + 1
	for i := 0; i < max; i++ {
		next, err := g.phi(current)
		if err != nil {
			return Labelling{}, err
		}
		// defensive monotonicity check
		if !LabellingLessOrEqual(current, next, univ) {
			return Labelling{}, fmt.Errorf("internal invariant: non-monotone phi at iteration %d", i)
		}
		// convergence by semantic equality over universe
		if next.Equals(current, univ) {
			return next, nil
		}
		current = next
	}
	return Labelling{}, fmt.Errorf("iteration did not converge within bound %d", max)
}
