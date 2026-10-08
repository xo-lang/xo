package rt

import (
	"iter"
	"slices"
	"strings"
	"sync/atomic"
)

// List is the immutable persistent vector `List[T]` (core 2.1, 5.2).
//
// Lists share backing arrays. `used` counts the elements of the shared
// array that some list already owns; a push appends in place only when the
// list ends exactly at `used` (claimed with a compare and swap), otherwise
// it copies. So `xs.push(x)` in a loop is amortized O(1), and every older
// list value keeps seeing exactly its own elements.
type List[T any] struct {
	s    []T
	used *atomic.Int64
}

// ListOf builds a list from elements.
func ListOf[T any](xs ...T) List[T] {
	if len(xs) == 0 {
		return List[T]{}
	}
	u := new(atomic.Int64)
	u.Store(int64(len(xs)))
	return List[T]{s: xs, used: u}
}

// ListFromSlice wraps a slice the caller no longer uses.
func ListFromSlice[T any](xs []T) List[T] { return ListOf(xs...) }

// Slice returns the elements; callers must not modify them.
func (l List[T]) Slice() []T { return l.s }

// M_len is `xs.len()`.
func (l List[T]) M_len(*Task) int64 { return int64(len(l.s)) }

// M_is_empty is `xs.is_empty()`.
func (l List[T]) M_is_empty(*Task) bool { return len(l.s) == 0 }

// At is `xs[i]`: faults out of range (core 3.6).
func (l List[T]) At(i int64, at string) T {
	if i < 0 || i >= int64(len(l.s)) {
		Faultf(at, "index %d out of range for List of length %d", i, len(l.s))
	}
	return l.s[i]
}

// M_get is `xs.get(i)`.
func (l List[T]) M_get(_ *Task, i int64) Option[T] {
	if i < 0 || i >= int64(len(l.s)) {
		return Option[T]{}
	}
	return Some(l.s[i])
}

// M_first is `xs.first()`.
func (l List[T]) M_first(tk *Task) Option[T] { return l.M_get(tk, 0) }

// M_last is `xs.last()`.
func (l List[T]) M_last(tk *Task) Option[T] { return l.M_get(tk, int64(len(l.s))-1) }

// All iterates the elements.
func (l List[T]) All() iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, x := range l.s {
			if !yield(x) {
				return
			}
		}
	}
}

// M_push is `xs.push(x)` on a `var` list.
func (l *List[T]) M_push(_ *Task, x T) {
	n := len(l.s)
	if l.used != nil && n < cap(l.s) && l.used.CompareAndSwap(int64(n), int64(n+1)) {
		l.s = l.s[:n+1]
		l.s[n] = x
		return
	}
	c := max(4, 2*n)
	s := make([]T, n+1, c)
	copy(s, l.s)
	s[n] = x
	u := new(atomic.Int64)
	u.Store(int64(n + 1))
	l.s, l.used = s, u
}

// M_pop is `xs.pop()`.
func (l *List[T]) M_pop(*Task) Option[T] {
	n := len(l.s)
	if n == 0 {
		return Option[T]{}
	}
	x := l.s[n-1]
	l.s = l.s[: n-1 : n-1]
	return Some(x)
}

func (l List[T]) copyCap(extra int) []T {
	s := make([]T, len(l.s), len(l.s)+extra)
	copy(s, l.s)
	return s
}

// M_insert is `xs.insert(i, x)`; i may equal the length.
func (l *List[T]) M_insert(_ *Task, i int64, x T) {
	if i < 0 || i > int64(len(l.s)) {
		FaultHere("index %d out of range for insert into List of length %d", i, len(l.s))
	}
	s := l.copyCap(1)
	s = slices.Insert(s, int(i), x)
	*l = ListOf(s...)
}

// M_remove_at is `xs.remove_at(i)`.
func (l *List[T]) M_remove_at(_ *Task, i int64) T {
	x := l.At(i, "")
	s := l.copyCap(0)
	s = slices.Delete(s, int(i), int(i)+1)
	*l = ListOf(s...)
	return x
}

// SetAt is `xs[i] = x`.
func (l *List[T]) SetAt(i int64, x T, at string) {
	if i < 0 || i >= int64(len(l.s)) {
		Faultf(at, "index %d out of range for List of length %d", i, len(l.s))
	}
	s := l.copyCap(0)
	s[i] = x
	*l = ListOf(s...)
}

// With returns a copy with element i replaced (used to write back through
// an index in a nested assignment).
func (l List[T]) With(i int64, x T, at string) List[T] {
	l.SetAt(i, x, at)
	return l
}

// SliceRange is `xs[lo..hi]` with -1 for an omitted bound (core 3.6).
func (l List[T]) SliceRange(lo, hi int64, hasLo, hasHi bool, at string) List[T] {
	n := int64(len(l.s))
	if !hasLo {
		lo = 0
	}
	if !hasHi {
		hi = n
	}
	if lo < 0 || hi > n || lo > hi {
		Faultf(at, "slice %d..%d out of range for List of length %d", lo, hi, n)
	}
	return List[T]{s: l.s[lo:hi:hi], used: l.used}
}

// Concat is `xs + ys`.
func (l List[T]) Concat(o List[T]) List[T] {
	if len(o.s) == 0 {
		return l
	}
	if len(l.s) == 0 {
		return o
	}
	r := l
	for _, x := range o.s {
		r.M_push(nil, x)
	}
	return r
}

// M_contains is `xs.contains(x)`.
func (l List[T]) M_contains(_ *Task, x T) bool {
	for _, y := range l.s {
		if Eq(x, y) {
			return true
		}
	}
	return false
}

// M_index_of is `xs.index_of(x)`.
func (l List[T]) M_index_of(_ *Task, x T) Option[int64] {
	for i, y := range l.s {
		if Eq(x, y) {
			return Some(int64(i))
		}
	}
	return Option[int64]{}
}

// List_indexed is `xs.indexed()` (a function: a method of List[T]
// returning List[(Int, T)] would be a Go instantiation cycle).
func List_indexed[T any](_ *Task, l List[T]) List[Tuple2[int64, T]] {
	out := make([]Tuple2[int64, T], len(l.s))
	for i, x := range l.s {
		out[i] = Tuple2[int64, T]{int64(i), x}
	}
	return ListOf(out...)
}

// M_reverse is `xs.reverse()`.
func (l List[T]) M_reverse(*Task) List[T] {
	out := make([]T, len(l.s))
	for i, x := range l.s {
		out[len(l.s)-1-i] = x
	}
	return ListOf(out...)
}

// M_sort is `xs.sort()`: stable, by the Ord of T.
func (l List[T]) M_sort(*Task) List[T] {
	out := slices.Clone(l.s)
	slices.SortStableFunc(out, func(a, b T) int { return Cmp(a, b) })
	return ListOf(out...)
}

// M_to_set is `xs.to_set()`.
func (l List[T]) M_to_set(*Task) Set[T] {
	var s Set[T]
	for _, x := range l.s {
		s.M_add(nil, x)
	}
	return s
}

// List_join is `xs.join(sep)` on a List[Str].
func List_join(_ *Task, l List[string], sep string) string { return strings.Join(l.s, sep) }

// List_map is `xs.map(f)`.
func List_map[T, U, E any](tk *Task, l List[T], f func(*Task, T) Result[U, E]) Result[List[U], E] {
	out := make([]U, 0, len(l.s))
	for _, x := range l.s {
		r := f(tk, x)
		if !r.Ok {
			return ErrTr[List[U]](r.E, r.Tr)
		}
		out = append(out, r.V)
	}
	return Ok[List[U], E](ListOf(out...))
}

// List_filter is `xs.filter(f)`.
func List_filter[T, E any](tk *Task, l List[T], f func(*Task, T) Result[bool, E]) Result[List[T], E] {
	var out []T
	for _, x := range l.s {
		r := f(tk, x)
		if !r.Ok {
			return ErrTr[List[T]](r.E, r.Tr)
		}
		if r.V {
			out = append(out, x)
		}
	}
	return Ok[List[T], E](ListOf(out...))
}

// List_fold is `xs.fold(init, f)`.
func List_fold[T, A, E any](tk *Task, l List[T], init A, f func(*Task, A, T) Result[A, E]) Result[A, E] {
	acc := init
	for _, x := range l.s {
		r := f(tk, acc, x)
		if !r.Ok {
			return ErrTr[A](r.E, r.Tr)
		}
		acc = r.V
	}
	return Ok[A, E](acc)
}

// List_find is `xs.find(f)`.
func List_find[T, E any](tk *Task, l List[T], f func(*Task, T) Result[bool, E]) Result[Option[T], E] {
	for _, x := range l.s {
		r := f(tk, x)
		if !r.Ok {
			return ErrTr[Option[T]](r.E, r.Tr)
		}
		if r.V {
			return Ok[Option[T], E](Some(x))
		}
	}
	return Ok[Option[T], E](Option[T]{})
}

// List_any is `xs.any(f)`.
func List_any[T, E any](tk *Task, l List[T], f func(*Task, T) Result[bool, E]) Result[bool, E] {
	for _, x := range l.s {
		r := f(tk, x)
		if !r.Ok {
			return ErrTr[bool](r.E, r.Tr)
		}
		if r.V {
			return Ok[bool, E](true)
		}
	}
	return Ok[bool, E](false)
}

// List_all is `xs.all(f)`.
func List_all[T, E any](tk *Task, l List[T], f func(*Task, T) Result[bool, E]) Result[bool, E] {
	for _, x := range l.s {
		r := f(tk, x)
		if !r.Ok {
			return ErrTr[bool](r.E, r.Tr)
		}
		if !r.V {
			return Ok[bool, E](false)
		}
	}
	return Ok[bool, E](true)
}

// List_sort_by is `xs.sort_by(key)`: stable.
func List_sort_by[T, K, E any](tk *Task, l List[T], key func(*Task, T) Result[K, E]) Result[List[T], E] {
	type kv struct {
		k K
		v T
	}
	tmp := make([]kv, len(l.s))
	for i, x := range l.s {
		r := key(tk, x)
		if !r.Ok {
			return ErrTr[List[T]](r.E, r.Tr)
		}
		tmp[i] = kv{r.V, x}
	}
	slices.SortStableFunc(tmp, func(a, b kv) int { return Cmp(a.k, b.k) })
	out := make([]T, len(tmp))
	for i, p := range tmp {
		out[i] = p.v
	}
	return Ok[List[T], E](ListOf(out...))
}

// XoEq implements Eq.
func (l List[T]) XoEq(o any) bool {
	m, ok := o.(List[T])
	if !ok || len(l.s) != len(m.s) {
		return false
	}
	for i := range l.s {
		if !Eq(l.s[i], m.s[i]) {
			return false
		}
	}
	return true
}

// XoHash implements Hash.
func (l List[T]) XoHash() uint64 {
	h := uint64(0x6c697374)
	for _, x := range l.s {
		h = mix(h, Hash(x))
	}
	return h
}

// XoCmp orders lists lexicographically.
func (l List[T]) XoCmp(o any) int {
	m := o.(List[T])
	for i := 0; i < len(l.s) && i < len(m.s); i++ {
		if c := Cmp(l.s[i], m.s[i]); c != 0 {
			return c
		}
	}
	return cmpInt(len(l.s), len(m.s))
}

// XoDebug implements Debug: `[1, 2, 3]`.
func (l List[T]) XoDebug(b *strings.Builder) {
	b.WriteByte('[')
	for i, x := range l.s {
		if i > 0 {
			b.WriteString(", ")
		}
		debugTo(b, x)
	}
	b.WriteByte(']')
}

// Buf is the move only buffer `Buf[T]` (core 5.3).
type Buf[T any] struct{ s []T }

// Buf_new is `Buf[T].new()`.
func Buf_new[T any](*Task) Buf[T] { return Buf[T]{} }

// Buf_with_capacity is `Buf[T].with_capacity(n)`.
func Buf_with_capacity[T any](_ *Task, n int64) Buf[T] { return Buf[T]{s: make([]T, 0, max(n, 0))} }

// M_push is `b.push(x)`.
func (b *Buf[T]) M_push(_ *Task, x T) { b.s = append(b.s, x) }

// M_len is `b.len()`.
func (b Buf[T]) M_len(*Task) int64 { return int64(len(b.s)) }

// Freeze is `b.freeze()`: moves the buffer into a list in O(1).
func (b Buf[T]) M_freeze(*Task) List[T] { return ListOf(b.s...) }

// All iterates the elements.
func (b Buf[T]) All() iter.Seq[T] { return List[T]{s: b.s}.All() }

// XoDebug implements Debug.
func (b Buf[T]) XoDebug(sb *strings.Builder) {
	sb.WriteString("Buf")
	List[T]{s: b.s}.XoDebug(sb)
}

// StrBuilder is the move only string builder (core 5.3).
type StrBuilder struct{ b []byte }

// StrBuilder_new is `StrBuilder.new()`.
func StrBuilder_new(*Task) StrBuilder { return StrBuilder{} }

// Write is `b.write(s)`.
func (b *StrBuilder) M_write(_ *Task, s string) { b.b = append(b.b, s...) }

// M_len is `b.len()`.
func (b StrBuilder) M_len(*Task) int64 { return int64(len(b.b)) }

// Freeze is `b.freeze()`.
func (b StrBuilder) M_freeze(*Task) string { return string(b.b) }

// XoDebug implements Debug.
func (b StrBuilder) XoDebug(sb *strings.Builder) {
	sb.WriteString("StrBuilder(")
	sb.WriteString(quote(string(b.b)))
	sb.WriteString(")")
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// M_set is `xs.set(i, x)`.
func (l *List[T]) M_set(_ *Task, i int64, x T) { l.SetAt(i, x, "") }

// Items returns the elements of a list for iteration; callers must not
// modify them.
func (l List[T]) Items() []T { return l.s }
