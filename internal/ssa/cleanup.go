package ssa

// cleanup runs after building: it prunes blocks unreachable from the
// entry, removes trivial block parameters (all incoming arguments are
// the parameter itself or one other value), and renumbers blocks and
// values in order so the printed form is stable.
func (f *Func) cleanup() {
	f.prune()
	f.removeTrivialParams()
	f.renumber()
}

func (f *Func) prune() {
	reach := map[*Block]bool{}
	var walk func(b *Block)
	walk = func(b *Block) {
		reach[b] = true
		if t := b.Term(); t != nil {
			for _, e := range t.Succs {
				if !reach[e.To] {
					walk(e.To)
				}
			}
		}
	}
	walk(f.Blocks[0])
	var keep []*Block
	for _, b := range f.Blocks {
		if !reach[b] {
			continue
		}
		var preds []*Edge
		for _, e := range b.Preds {
			if reach[e.From] {
				preds = append(preds, e)
			}
		}
		b.Preds = preds
		keep = append(keep, b)
	}
	f.Blocks = keep
}

func (f *Func) removeTrivialParams() {
	repl := map[*Value]*Value{}
	find := func(v *Value) *Value {
		for repl[v] != nil {
			v = repl[v]
		}
		return v
	}
	for changed := true; changed; {
		changed = false
		for _, b := range f.Blocks[1:] {
			for i := 0; i < len(b.Params); i++ {
				p := b.Params[i]
				var same *Value
				trivial := true
				for _, e := range b.Preds {
					a := find(e.Args[i])
					if a == p || a == same {
						continue
					}
					if same != nil {
						trivial = false
						break
					}
					same = a
				}
				if !trivial || same == nil {
					continue
				}
				repl[p] = same
				b.Params = append(b.Params[:i:i], b.Params[i+1:]...)
				for _, e := range b.Preds {
					e.Args = append(e.Args[:i:i], e.Args[i+1:]...)
				}
				i--
				changed = true
			}
		}
	}
	if len(repl) == 0 {
		return
	}
	for _, b := range f.Blocks {
		for _, v := range b.Instrs {
			for i, a := range v.Args {
				v.Args[i] = find(a)
			}
			for _, e := range v.Succs {
				for i, a := range e.Args {
					e.Args[i] = find(a)
				}
			}
		}
	}
}

func (f *Func) renumber() {
	n := 0
	for i, b := range f.Blocks {
		b.ID = i
		for _, p := range b.Params {
			p.ID = n
			n++
		}
		for _, v := range b.Instrs {
			v.ID = n
			n++
		}
	}
	f.nextBlock = len(f.Blocks)
	f.nextValue = n
}
