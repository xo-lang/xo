package ssa

// CancelledRelease reports the release that retain v cancels, or nil:
// v retains the one heap field of the variant its block is known to
// hold, and a release later in the block drops that enum value with
// nothing in between but pure projections and constructors (`if let
// Some(w) = xs.pop()` retains w and releases the Option). A backend may
// then emit neither: the release would drop exactly the reference the
// retain added. This holds for counts shared between threads too
// (decision 0058): the pair changes no count.
func CancelledRelease(v *Value) *Value {
	if v.Op != OpRetain {
		return nil
	}
	x := v.Args[0]
	if x.Op != OpPayload || (x.Type.Kind != KStr && x.Type.Kind != KList) {
		return nil
	}
	ref := x.Aux.(PayloadRef)
	e := x.Args[0]
	if e.Type.Kind != KEnum || len(e.Type.Variants[ref.Tag].Fields) != 1 {
		return nil
	}
	// the block runs only when e has the variant: its one predecessor
	// branches on tag(e) == Tag
	b := v.Block
	if len(b.Preds) != 1 {
		return nil
	}
	br := b.Preds[0].From.Term()
	if br == nil || br.Op != OpBranch || br.Succs[0].To == br.Succs[1].To || br.Succs[0].To != b {
		return nil
	}
	c := br.Args[0]
	if c.Op != OpEq || c.Args[0].Op != OpTag || !sameProjection(c.Args[0].Args[0], e) {
		return nil
	}
	if k, ok := c.Args[1].Aux.(int64); !ok || c.Args[1].Op != OpConst || k != int64(ref.Tag) {
		return nil
	}
	i := 0
	for i < len(b.Instrs) && b.Instrs[i] != v {
		i++
	}
	for _, w := range b.Instrs[i+1:] {
		switch w.Op {
		case OpField, OpTag, OpPayload, OpConst, OpTuple, OpStruct:
			continue
		case OpRelease:
			if sameProjection(w.Args[0], e) {
				return w
			}
		}
		return nil
	}
	return nil
}

// sameProjection reports whether a and b are the same value read
// through the same chain of field projections.
func sameProjection(a, b *Value) bool {
	for a.Op == OpField && b.Op == OpField {
		if a.Aux.(int) != b.Aux.(int) {
			return false
		}
		a, b = a.Args[0], b.Args[0]
	}
	return a == b
}
