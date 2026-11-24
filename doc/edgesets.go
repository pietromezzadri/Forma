package doc

import (
	"fmt"
	"math"
	"sort"
)

// Edge sets: a fillet or chamfer step rounds its edges with its own Radius
// (Distance), and any set of them with a size of their own (Fusion's
// fillet sets): edge set k (EdgeRef.Set) takes the input "Radius k+1"
// ("Distance k+1"). Where three rounds of one size meet at a corner it is
// a ball; where sizes differ the rounds cross in a mitre.

// SizeKey is the input holding edge set k's size in step f.
func SizeKey(f *Feature, set int) string {
	base := "Radius"
	if f.Kind == KChamfer {
		base = "Distance"
	}
	if set == 0 {
		return base
	}
	return fmt.Sprintf("%s %d", base, set+1)
}

// EdgeSets are the step's edge sets with a size of their own, in order.
func EdgeSets(f *Feature) []int {
	seen := map[int]bool{}
	var out []int
	for _, r := range f.Edges {
		if r.Set > 0 && !seen[r.Set] {
			seen[r.Set] = true
			out = append(out, r.Set)
		}
	}
	sort.Ints(out)
	return out
}

// SetEdgeSize gives the step's edges at indices idx (already in it) and
// the edges add (put in) a size of their own, expr, as a new set; its
// number comes back. An edge of add the step already has (the same faces
// either side, the same place) is moved, not added twice.
func SetEdgeSize(f *Feature, idx []int, add []EdgeRef, expr string) int {
	set := 1
	for _, r := range f.Edges {
		if r.Set >= set {
			set = r.Set + 1
		}
	}
	for _, i := range idx {
		if i >= 0 && i < len(f.Edges) {
			f.Edges[i].Set = set
		}
	}
next:
	for _, r := range add {
		for i, o := range f.Edges {
			if sameEdgeRef(o, r) {
				f.Edges[i].Set = set
				continue next
			}
		}
		r.Set = set
		f.Edges = append(f.Edges, r)
	}
	if f.Inputs == nil {
		f.Inputs = map[string]string{}
	}
	f.Inputs[SizeKey(f, set)] = expr
	return set
}

// ClearEdgeSet puts set's edges back on the step's own size.
func ClearEdgeSet(f *Feature, set int) {
	for i := range f.Edges {
		if f.Edges[i].Set == set {
			f.Edges[i].Set = 0
		}
	}
	delete(f.Inputs, SizeKey(f, set))
}

func sameEdgeRef(a, b EdgeRef) bool {
	faces := (a.A == b.A && a.B == b.B) || (a.A == b.B && a.B == b.A)
	return faces && a.P.Dist(b.P) < 1e-6*(1+math.Max(a.P.Len(), b.P.Len()))
}
