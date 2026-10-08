package ssa

// InsertShareProbes marks every heap value shared right after it is
// defined (OpShare after each instruction and at the start of each block
// for its parameters), as if each crossed to another thread at once. It
// runs before InsertRC. A test hook (decision 0058): no construct lowers
// to OpShare yet, and a program built with probes must behave exactly as
// without them, with every count update taking the atomic path, every
// in place update going through the shared claim, and the generated
// share walkers visiting every type.
func (m *Module) InsertShareProbes() {
	for _, f := range m.Funcs {
		for _, b := range f.Blocks {
			var out []*Value
			probe := func(v *Value) {
				if v.Type == nil || !v.Type.Heap() || v.Op == OpUndef {
					return
				}
				s := f.newValue(OpShare, nil, []*Value{v}, nil)
				s.Block = b
				out = append(out, s)
			}
			for _, p := range b.Params {
				probe(p)
			}
			for _, v := range b.Instrs {
				out = append(out, v)
				if v.Op.IsTerminator() {
					continue
				}
				probe(v)
			}
			b.Instrs = out
		}
	}
}
