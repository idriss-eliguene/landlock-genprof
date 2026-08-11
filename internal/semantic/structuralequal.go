package semantic

// StructuralEqual implements RFC-0002's structural equality over the
// SnapshotValue domain. It is intentionally conservative and explicit:
// atomics compare by identity token; literals by kind+value; tuples by
// length and elementwise StructuralEqual; sets by mathematical-set
// equality using StructuralEqual on members; records by same field set
// and StructuralEqual on each field; propositions by predicate name and
// positional arg equality (set-valued args must be provided as Set types
// to indicate unordered semantics).

func StructuralEqual(a, b SnapshotValue) bool {
	// nil/nil
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	switch xa := a.(type) {
	case Literal:
		yb, ok := b.(Literal)
		if !ok {
			return false
		}
		return xa.kind == yb.kind && xa.value == yb.value
	case IdentityRef:
		yb, ok := b.(IdentityRef)
		if !ok {
			return false
		}
		// compare token and typeName
		return xa.token == yb.token && xa.typeName == yb.typeName
	case Tuple:
		yb, ok := b.(Tuple)
		if !ok {
			return false
		}
		if len(xa.elems) != len(yb.elems) {
			return false
		}
		for i := range xa.elems {
			if !StructuralEqual(xa.elems[i], yb.elems[i]) {
				return false
			}
		}
		return true
	case Set:
		yb, ok := b.(Set)
		if !ok {
			return false
		}
		// mathematical set equality: for every member in xa there exists a
		// structurally equal member in yb and sizes equal
		if len(xa.members) != len(yb.members) {
			return false
		}
		matched := make([]bool, len(yb.members))
		for _, m := range xa.members {
			found := false
			for j, n := range yb.members {
				if matched[j] {
					continue
				}
				if StructuralEqual(m, n) {
					matched[j] = true
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	case Record:
		yb, ok := b.(Record)
		if !ok {
			return false
		}
		// compare keys
		if len(xa.keys) != len(yb.keys) {
			return false
		}
		for i := range xa.keys {
			if xa.keys[i] != yb.keys[i] {
				return false
			}
			// compare field values
			xv := xa.fields[xa.keys[i]]
			yv := yb.fields[yb.keys[i]]
			if !StructuralEqual(xv, yv) {
				return false
			}
		}
		return true
	case Proposition:
		yb, ok := b.(Proposition)
		if !ok {
			return false
		}
		// compare phase (IdentityRef)
		if !StructuralEqual(xa.phase, yb.phase) {
			return false
		}
		if xa.modality != yb.modality {
			return false
		}
		// term identity (IdentityRef)
		if !StructuralEqual(xa.term, yb.term) {
			return false
		}
		if xa.quantification != yb.quantification {
			return false
		}
		// validTime equality: compare start/end pointers
		if (xa.validTime.Start == nil) != (yb.validTime.Start == nil) {
			return false
		}
		if xa.validTime.Start != nil && yb.validTime.Start != nil {
			if !xa.validTime.Start.Equal(*yb.validTime.Start) {
				return false
			}
		}
		if (xa.validTime.End == nil) != (yb.validTime.End == nil) {
			return false
		}
		if xa.validTime.End != nil && yb.validTime.End != nil {
			if !xa.validTime.End.Equal(*yb.validTime.End) {
				return false
			}
		}
		if len(xa.args) != len(yb.args) {
			return false
		}
		for i := range xa.args {
			if !StructuralEqual(xa.args[i], yb.args[i]) {
				return false
			}
		}
		return true
	default:
		// Unknown SnapshotValue implementation — conservative false
		return false
	}
}
