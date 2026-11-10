package rt

import "slices"

// Ordering's methods and the comparator sorts (decision 0586). Ordering is
// the prelude enum generated in std_gen_prelude.go: Less, Equal and
// Greater are tags 1, 2 and 3.

// OrdCmp is `a.cmp(b)` on an Ord type.
func OrdCmp[T any](a, b T) Ordering {
	c := Cmp(a, b)
	switch {
	case c < 0:
		return Ordering{Tag: 1}
	case c > 0:
		return Ordering{Tag: 3}
	}
	return Ordering{Tag: 2}
}

// M_reverse is `o.reverse()`.
func (o Ordering) M_reverse(*Task) Ordering { return Ordering{Tag: 4 - o.Tag} }

// M_then is `o.then(next)`.
func (o Ordering) M_then(_ *Task, next Ordering) Ordering {
	if o.Tag == 2 {
		return next
	}
	return o
}

// sortPermWith is the stable order of s's positions by cmp; the first
// error stops the comparisons (the rest compare equal) and is returned.
func sortPermWith[T, E any](tk *Task, s []T, cmp func(*Task, T, T) Result[Ordering, E]) ([]int32, *Result[Unit, E]) {
	perm := make([]int32, len(s))
	for i := range perm {
		perm[i] = int32(i)
	}
	var bad *Result[Unit, E]
	slices.SortStableFunc(perm, func(a, b int32) int {
		if bad != nil {
			return 0
		}
		r := cmp(tk, s[a], s[b])
		if !r.Ok {
			e := ErrTr[Unit](r.E, r.Tr)
			bad = &e
			return 0
		}
		return r.V.Tag - 2
	})
	return perm, bad
}

// List_sort_with is `xs.sort_with(cmp)`: stable.
func List_sort_with[T, E any](tk *Task, l List[T], cmp func(*Task, T, T) Result[Ordering, E]) Result[List[T], E] {
	perm, bad := sortPermWith(tk, l.s, cmp)
	if bad != nil {
		return ErrTr[List[T]](bad.E, bad.Tr)
	}
	out := make([]T, len(perm))
	for i, j := range perm {
		out[i] = l.s[j]
	}
	markElems(out)
	return Ok[List[T], E](ListOf(out...))
}

// List_sort_in_place_with is `xs.sort_in_place_with(cmp)`.
func List_sort_in_place_with[T, E any](tk *Task, l *List[T], cmp func(*Task, T, T) Result[Ordering, E]) Result[Unit, E] {
	if len(l.s) < 2 {
		return Ok[Unit, E](Unit{})
	}
	perm, bad := sortPermWith(tk, l.s, cmp)
	if bad != nil {
		return *bad
	}
	List_own(l, 0)
	out := make([]T, len(l.s))
	for i, j := range perm {
		out[i] = l.s[j]
	}
	l.s, l.b = out, newBlock(len(out))
	markElems(l.s)
	return Ok[Unit, E](Unit{})
}

// descPerm is the stable order of the positions by keys, greatest first.
func descPerm[K any](keys []K) []int32 {
	perm := make([]int32, len(keys))
	for i := range perm {
		perm[i] = int32(i)
	}
	slices.SortStableFunc(perm, func(a, b int32) int { return Cmp(keys[b], keys[a]) })
	return perm
}

// List_sort_by_desc is `xs.sort_by_desc(key)`: stable.
func List_sort_by_desc[T, K, E any](tk *Task, l List[T], key func(*Task, T) Result[K, E]) Result[List[T], E] {
	keys := make([]K, len(l.s))
	for i, x := range l.s {
		r := key(tk, x)
		if !r.Ok {
			return ErrTr[List[T]](r.E, r.Tr)
		}
		keys[i] = r.V
	}
	perm := descPerm(keys)
	out := make([]T, len(perm))
	for i, j := range perm {
		out[i] = l.s[j]
	}
	markElems(out)
	return Ok[List[T], E](ListOf(out...))
}

// List_sort_in_place_by_desc is `xs.sort_in_place_by_desc(key)`.
func List_sort_in_place_by_desc[T, K, E any](tk *Task, l *List[T], key func(*Task, T) Result[K, E]) Result[Unit, E] {
	if len(l.s) < 2 {
		return Ok[Unit, E](Unit{})
	}
	keys := make([]K, len(l.s))
	for i, x := range l.s {
		r := key(tk, x)
		if !r.Ok {
			return ErrTr[Unit](r.E, r.Tr)
		}
		keys[i] = r.V
	}
	List_own(l, 0)
	perm := descPerm(keys)
	out := make([]T, len(l.s))
	for i, j := range perm {
		out[i] = l.s[j]
	}
	l.s, l.b = out, newBlock(len(out))
	markElems(l.s)
	return Ok[Unit, E](Unit{})
}
