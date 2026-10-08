package rt

import (
	"iter"
	"slices"
	"strings"
	"sync/atomic"
)

// List is the persistent vector `List[T]` (core 2.1, 5.2) with in place
// writes for the unique owner.
//
// A list value is a header: a Go slice (its elements are the slice's
// length prefix of a backing array) and a pointer to the array's block.
// The block holds two facts about the backing array:
//
//   - used counts the elements of the array that some header already
//     covers. A push appends in place only when the header ends exactly at
//     used (claimed with a compare and swap), otherwise it copies. Slots at
//     or past used are seen by no header, so an append never changes what
//     another header sees, shared or not.
//   - shared is a sticky flag: once set, the array may be seen by more
//     than one header, and its elements (positions below used) are never
//     written again. A write through a header of a shared (or nil, for
//     arena memory) block first copies the elements into a fresh block
//     owned by that header; the copy marks the elements it duplicates
//     (MarkP), because their own lists are now reachable from two arrays.
//
// Element writes (`xs[i] = x`, `xs.set`, `xs[i].f = x`, see Mine and
// PtrM), `pop`, `insert`, and `remove_at` work in place on an unshared
// block, so they cost what they cost on a Go slice. Who sets shared:
//
//   - the generated code, wherever a list value is copied out of a place
//     (a binding, field, or element) into anything that outlives the read:
//     another binding, a field, a collection, a closure capture, a return,
//     a channel send, a call of std code. It wraps the read in Own (or,
//     for a struct, tuple, or Option that holds lists, a function literal
//     calling XoMark), which marks the blocks the value holds
//     (internal/gen/own.go). Results of std calls are marked the same way,
//     except fresh producers that mark the elements they copy.
//   - the runtime, wherever it returns a header sharing a block with an
//     argument (SliceRange, Concat) or copies elements of one list into
//     another list or collection (markElems).
//
// Soundness (value semantics, core 5.1): no header ever observes a write
// made through another header. Invariant U: a block whose shared flag is
// clear is reachable from exactly one live header, and that header lives
// in a place that only its owner's code reads (a binding, a field or
// element of an owned value, or a temporary of the generated write path).
// U holds for a fresh block (ListOf, a copy, a growth). Making a second
// header for a block means reading the first one out of its place; every
// such read either marks the block (an own site) or is a borrow: a read
// whose copy dies before the place can be written again (an index base,
// the receiver of a read only method, an argument of a call that cannot
// write the place, a loop subject the body does not write). A list nested
// in an element is protected lazily: while the outer block is unshared
// the inner header is reachable only through it; when the outer block is
// copied, markElems marks every inner block. So a write through an
// unshared block cannot be seen by any other header, and a write through
// a shared one copies first. Blocks are only ever marked, never unmarked,
// so concurrent readers of a shared block (other tasks) only race with
// appends past their own length, which they never read.
type List[T any] struct {
	s []T
	b *listBlock
}

// listBlock describes one backing array (see List).
//
// While shared is false only the one header that owns the block (and the
// task running its code) writes it, so used is plain memory then: the
// owner reads and writes it without atomics. Another task only gets a
// header after the block was marked (spawn captures, channel sends,
// Mutex.with all mark first, and a read of a value the runtime hands to
// several tasks, such as Once.get, is marked by the reader); from then on
// used changes only by compare and swap. A data race report on used
// under -race means invariant U was broken. shared is atomic because
// several tasks may mark the same block (each reader of a Once value).
type listBlock struct {
	used   int64
	shared atomic.Bool
}

func newBlock(n int) *listBlock { return &listBlock{used: int64(n)} }

// marker is implemented by values that hold lists: List, Option, Result,
// tuples, and generated structs and newtypes whose fields hold lists.
// XoMark marks shared the blocks of the lists the value holds inline
// (lists inside those lists are marked lazily, see List).
type marker interface{ XoMark() }

// MarkP marks the lists held by *p (a no op for types without lists).
func MarkP[T any](p *T) {
	if m, ok := any(p).(marker); ok {
		m.XoMark()
	}
}

// Own marks the lists held by v and returns it: generated code wraps a
// value of a type parameter in Own where it copies it out of a place.
//
// Only types that hold lists take the marking path, through a heap copy:
// MarkP(&v) moved v to the heap on every call, an allocation for every
// type (escape analysis does not see that markable is false).
func Own[T any](v T) T {
	if !markable[T]() {
		return v
	}
	p := new(T)
	*p = v
	MarkP(p)
	return *p
}

// markable reports whether values of T may hold lists.
func markable[T any]() bool {
	_, ok := any((*T)(nil)).(marker)
	return ok
}

// markElems marks the lists held by the elements of s: s was copied from
// (or into) another array, so those lists are now reachable twice.
func markElems[T any](s []T) {
	if len(s) == 0 || !markable[T]() {
		return
	}
	for i := range s {
		MarkP(&s[i])
	}
}

// XoMark marks the list's block shared.
func (l *List[T]) XoMark() { l.markShared() }

// Own marks the list's block shared and returns the list (an own site in
// generated code).
func (l List[T]) Own() List[T] {
	l.markShared()
	return l
}

func (l List[T]) markShared() {
	if b := l.b; b != nil && !b.shared.Load() {
		b.shared.Store(true)
	}
}

// unique reports whether l may write its elements in place.
func (l *List[T]) unique() bool {
	b := l.b
	return b != nil && !b.shared.Load()
}

// ListOf builds a list from elements; the caller gives up xs.
func ListOf[T any](xs ...T) List[T] {
	if len(xs) == 0 {
		return List[T]{}
	}
	return List[T]{s: xs, b: newBlock(len(xs))}
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

// own makes l the unique owner of its elements: a no op when it already
// is, else a copy into a fresh block with capacity for extra more
// elements (marking the duplicated elements).
func (l *List[T]) own(extra int) {
	if l.unique() && len(l.s)+extra <= cap(l.s) {
		return
	}
	l.copyOut(extra)
}

// copyOut copies the elements into a fresh unshared block with room for
// extra more. If l's block was unique, the old array is dropped and its
// elements move; otherwise they are duplicated and marked.
func (l *List[T]) copyOut(extra int) {
	n := len(l.s)
	c := n + extra
	if extra > 0 {
		c = max(c, 4, 2*n)
	}
	s := make([]T, n, c)
	copy(s, l.s)
	if !l.unique() {
		markElems(s)
	}
	l.s, l.b = s, newBlock(n)
}

// SetAt is `xs[i] = x`: in place when l is the unique owner.
func (l *List[T]) SetAt(i int64, x T, at string) {
	if uint64(i) >= uint64(len(l.s)) {
		panic(&arithFault{i, int64(len(l.s)), 'i', at})
	}
	if b := l.b; b == nil || b.shared.Load() {
		l.copyOut(0)
	}
	l.s[i] = x
}

// Inline write forms. Generated code writes element i of a list place p
// as
//
//	if !p.Mine() { p = p.Unshared() }
//	p.PutM(i, x, at)        // or p.PtrM(i, at) for a write below i
//
// and pushes as
//
//	if p.CanPush() { p = p.PushFast(x) } else { p = p.PushSlow(x) }
//
// Each method inlines, and none takes the address of p, so a local list
// stays in registers (SetAt and M_push are over Go's inlining budget for
// generic shapes, and their pointer receivers keep the binding in memory).

// Mine reports whether l is the unique owner of its array: it may write
// its elements in place.
func (l List[T]) Mine() bool {
	b := l.b
	return b != nil && !b.shared.Load()
}

// Unshared returns l as the unique owner of a copy of its elements (see
// copyOut).
func (l List[T]) Unshared() List[T] {
	l.copyOut(0)
	return l
}

// PutM stores x at i (faults out of range). l must be the unique owner
// (Mine): the store goes to the array l shares with its place.
func (l List[T]) PutM(i int64, x T, at string) {
	if uint64(i) >= uint64(len(l.s)) {
		panic(&arithFault{i, int64(len(l.s)), 'i', at})
	}
	l.s[i] = x
}

// PtrM is the address of element i (faults out of range). l must be the
// unique owner (Mine).
func (l List[T]) PtrM(i int64, at string) *T {
	if uint64(i) >= uint64(len(l.s)) {
		panic(&arithFault{i, int64(len(l.s)), 'i', at})
	}
	return &l.s[i]
}

// CanPush reports whether PushFast applies: l is the unique owner and its
// array has room.
func (l List[T]) CanPush() bool {
	b := l.b
	n := len(l.s)
	// shared first: once it is set, used may change under another task
	return b != nil && n < cap(l.s) && !b.shared.Load() && b.used == int64(n)
}

// PushFast is l with x appended in place (CanPush must hold).
func (l List[T]) PushFast(x T) List[T] {
	n := len(l.s)
	l.b.used = int64(n + 1)
	l.s = l.s[:n+1]
	l.s[n] = x
	return l
}

// PushSlow is l with x appended (M_push).
func (l List[T]) PushSlow(x T) List[T] {
	l.M_push(nil, x)
	return l
}

// M_set is `xs.set(i, x)`: the fault is located at the caller's line.
func (l *List[T]) M_set(_ *Task, i int64, x T) {
	if i < 0 || i >= int64(len(l.s)) {
		Faultf("", "index %d out of range for List of length %d", i, len(l.s))
	}
	l.SetAt(i, x, "")
}

// With returns a copy with element i replaced; l is unchanged.
func (l List[T]) With(i int64, x T, at string) List[T] {
	l.markShared()
	l.SetAt(i, x, at)
	return l
}

// M_push is `xs.push(x)` on a `var` list: amortized O(1). It appends in
// place when l ends at the block's claimed length (shared or not: no
// header sees that slot), else it copies.
func (l *List[T]) M_push(_ *Task, x T) {
	n := len(l.s)
	if b := l.b; b != nil && n < cap(l.s) {
		shared := b.shared.Load()
		if !shared && b.used == int64(n) {
			// the unique owner: no other header can claim
			b.used = int64(n + 1)
			l.s = l.s[:n+1]
			l.s[n] = x
			return
		}
		if shared && atomic.CompareAndSwapInt64(&b.used, int64(n), int64(n+1)) {
			l.s = l.s[:n+1]
			l.s[n] = x
			return
		}
	}
	l.copyOut(1)
	l.b.used = int64(n + 1)
	l.s = l.s[:n+1]
	l.s[n] = x
}

// Push1 appends x in place when l is the unique owner and has room, and
// reports whether it did. Generated code pushes as
// `if !xs.Push1(x) { xs.M_push(tk, x) }`, which inlines the fast path.
func (l *List[T]) Push1(x T) bool {
	b := l.b
	n := len(l.s)
	// shared first: once it is set, used may change under another task
	if b == nil || n >= cap(l.s) || b.shared.Load() || b.used != int64(n) {
		return false
	}
	b.used = int64(n + 1)
	l.s = l.s[:n+1]
	l.s[n] = x
	return true
}

// M_pop is `xs.pop()`. The unique owner gives the slot back (the next
// push reuses it); a shared list only shortens its own view.
func (l *List[T]) M_pop(*Task) Option[T] {
	n := len(l.s)
	if n == 0 {
		return Option[T]{}
	}
	x := l.s[n-1]
	if l.unique() && l.b.used == int64(n) {
		var z T
		l.s[n-1] = z
		l.b.used = int64(n - 1)
	}
	l.s = l.s[:n-1]
	return Some(x)
}

// M_insert is `xs.insert(i, x)`; i may equal the length. In place for the
// unique owner (a shift, like a Go slice insert).
func (l *List[T]) M_insert(_ *Task, i int64, x T) {
	n := len(l.s)
	if i < 0 || i > int64(n) {
		FaultHere("index %d out of range for insert into List of length %d", i, n)
	}
	l.own(1)
	l.b.used = int64(n + 1)
	l.s = l.s[:n+1]
	copy(l.s[i+1:], l.s[i:n])
	l.s[i] = x
}

// M_remove_at is `xs.remove_at(i)`. In place for the unique owner (a
// shift, like a Go slice delete).
func (l *List[T]) M_remove_at(_ *Task, i int64) T {
	x := l.At(i, "")
	n := len(l.s)
	if !l.unique() {
		s := make([]T, n-1, max(n-1, 4))
		copy(s, l.s[:i])
		copy(s[i:], l.s[i+1:])
		markElems(s)
		l.s, l.b = s, newBlock(n-1)
		return x
	}
	copy(l.s[i:], l.s[i+1:])
	var z T
	l.s[n-1] = z
	l.s = l.s[:n-1]
	l.b.used = int64(n - 1)
	return x
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
	// the slice shares l's array
	l.markShared()
	return List[T]{s: l.s[lo:hi:hi], b: l.b}
}

// Concat is `xs + ys`. The result may share l's or o's array, so both
// are marked; o's elements are duplicated into the result, so they are
// marked too.
func (l List[T]) Concat(o List[T]) List[T] {
	l.markShared()
	o.markShared()
	if len(o.s) == 0 {
		return l
	}
	if len(l.s) == 0 {
		return o
	}
	markElems(o.s)
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
	markElems(l.s)
	return ListOf(out...)
}

// M_reverse is `xs.reverse()`.
func (l List[T]) M_reverse(*Task) List[T] {
	out := make([]T, len(l.s))
	for i, x := range l.s {
		out[len(l.s)-1-i] = x
	}
	markElems(out)
	return ListOf(out...)
}

// M_sort is `xs.sort()`: stable, by the Ord of T.
func (l List[T]) M_sort(*Task) List[T] {
	out := slices.Clone(l.s)
	slices.SortStableFunc(out, func(a, b T) int { return Cmp(a, b) })
	markElems(out)
	return ListOf(out...)
}

// M_to_set is `xs.to_set()`.
func (l List[T]) M_to_set(*Task) Set[T] {
	var s Set[T]
	markElems(l.s)
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
	markElems(out)
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
	markElems(out)
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

// Items returns the elements of a list for iteration; callers must not
// modify them.
func (l List[T]) Items() []T { return l.s }
