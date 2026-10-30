package rt

import (
	"iter"
	"slices"
	"strings"
	"sync/atomic"
	"unicode/utf8"
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

// Addr is a pointer to a copy of v: the generated code calls a Go method
// on a receiver that is not addressable (an own-marked copy, decision
// 0335) through it, since a std type's method may have a pointer
// receiver (Http_Request.M_client_ip, Embed_Dir.M_paths).
func Addr[T any](v T) *T { return &v }

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
func (l *List[T]) XoMark() { List_markShared(*l) }

// List_Own marks the list's block shared and returns the list (an own site in
// generated code).
func List_Own[T any](l List[T]) List[T] {
	List_markShared(l)
	return l
}

func List_markShared[T any](l List[T]) {
	if b := l.b; b != nil && !b.shared.Load() {
		b.shared.Store(true)
	}
}

// List_unique reports whether l may write its elements in place.
func List_unique[T any](l *List[T]) bool {
	b := l.b
	return b != nil && !b.shared.Load()
}

// ListOf builds a list from elements; the caller gives up xs. It has no
// block (2026-10-21): a nil block means "not known to be unique", so the
// first write copies (List_copyOut gives the copy a block), which a push
// does anyway (xs has no room past its length), and a list that is only
// read (a split's parts, a map's keys) never allocates one. That was one
// allocation of every fresh list from std/rt (ADR 0312's path profile:
// 3 of the 15 an iteration).
func ListOf[T any](xs ...T) List[T] {
	if len(xs) == 0 {
		return List[T]{}
	}
	return List[T]{s: xs}
}

// ListFromSlice wraps a slice the caller no longer uses.
func ListFromSlice[T any](xs []T) List[T] { return ListOf(xs...) }

// List_Slice returns the elements; callers must not modify them.
func List_Slice[T any](l List[T]) []T { return l.s }

// List_len is `xs.len()`.
func List_len[T any](_ *Task, l List[T]) int64 { return int64(len(l.s)) }

// List_is_empty is `xs.is_empty()`.
func List_is_empty[T any](_ *Task, l List[T]) bool { return len(l.s) == 0 }

// List_At is `xs[i]`: faults out of range (core 3.6).
func List_At[T any](l List[T], i int64, at string) T {
	if i < 0 || i >= int64(len(l.s)) {
		Faultf(at, "index %d out of range for List of length %d", i, len(l.s))
	}
	return l.s[i]
}

// List_get is `xs.get(i)`.
func List_get[T any](_ *Task, l List[T], i int64) Option[T] {
	if i < 0 || i >= int64(len(l.s)) {
		return Option[T]{}
	}
	return Some(l.s[i])
}

// List_first is `xs.first()`.
func List_first[T any](tk *Task, l List[T]) Option[T] { return List_get(tk, l, 0) }

// List_last is `xs.last()`.
func List_last[T any](tk *Task, l List[T]) Option[T] { return List_get(tk, l, int64(len(l.s))-1) }

// List_All iterates the elements.
func List_All[T any](l List[T]) iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, x := range l.s {
			if !yield(x) {
				return
			}
		}
	}
}

// List_own makes l the unique owner of its elements: a no op when it already
// is, else a copy into a fresh block with capacity for extra more
// elements (marking the duplicated elements).
func List_own[T any](l *List[T], extra int) {
	if List_unique(l) && len(l.s)+extra <= cap(l.s) {
		return
	}
	List_copyOut(l, extra)
}

// List_copyOut copies the elements into a fresh unshared block with room for
// extra more. If l's block was unique, the old array is dropped and its
// elements move; otherwise they are duplicated and marked.
func List_copyOut[T any](l *List[T], extra int) {
	n := len(l.s)
	c := n + extra
	if extra > 0 {
		c = max(c, 4, 2*n)
	}
	s := make([]T, n, c)
	copy(s, l.s)
	if List_unique(l) {
		// the unique owner grows: no other header reaches the block
		// (invariant U), so it describes the new array (one allocation
		// fewer a growth)
		l.s = s
		l.b.used = int64(n)
		return
	}
	markElems(s)
	l.s, l.b = s, newBlock(n)
}

// List_SetAt is `xs[i] = x`: in place when l is the unique owner.
func List_SetAt[T any](l *List[T], i int64, x T, at string) {
	if uint64(i) >= uint64(len(l.s)) {
		panic(&arithFault{i, int64(len(l.s)), 'i', at})
	}
	if b := l.b; b == nil || b.shared.Load() {
		List_copyOut(l, 0)
	}
	l.s[i] = x
}

// Inline write forms. Generated code writes element i of a list place p
// as
//
//	if !List_Mine(p) { p = List_Unshared(p) }
//	List_PutM(p, i, x, at)        // or List_PtrM(p, i, at) for a write below i
//
// and pushes as
//
//	if List_CanPush(p) { p = List_PushFast(p, x) } else { p = List_PushSlow(p, x) }
//
// Each function inlines, and none takes the address of p, so a local list
// stays in registers (SetAt and M_push are over Go's inlining budget for
// generic shapes, and their pointer receivers keep the binding in memory).

// List_Mine reports whether l is the unique owner of its array: it may write
// its elements in place.
func List_Mine[T any](l List[T]) bool {
	b := l.b
	return b != nil && !b.shared.Load()
}

// List_Unshared returns l as the unique owner of a copy of its elements (see
// copyOut).
func List_Unshared[T any](l List[T]) List[T] {
	List_copyOut(&l, 0)
	return l
}

// List_PutM stores x at i (faults out of range). l must be the unique owner
// (Mine): the store goes to the array l shares with its place.
func List_PutM[T any](l List[T], i int64, x T, at string) {
	if uint64(i) >= uint64(len(l.s)) {
		panic(&arithFault{i, int64(len(l.s)), 'i', at})
	}
	l.s[i] = x
}

// List_PtrM is the address of element i (faults out of range). l must be the
// unique owner (Mine).
func List_PtrM[T any](l List[T], i int64, at string) *T {
	if uint64(i) >= uint64(len(l.s)) {
		panic(&arithFault{i, int64(len(l.s)), 'i', at})
	}
	return &l.s[i]
}

// List_CanPush reports whether PushFast applies: l is the unique owner and its
// array has room.
func List_CanPush[T any](l List[T]) bool {
	b := l.b
	n := len(l.s)
	// shared first: once it is set, used may change under another task
	return b != nil && n < cap(l.s) && !b.shared.Load() && b.used == int64(n)
}

// List_PushFast is l with x appended in place (CanPush must hold).
func List_PushFast[T any](l List[T], x T) List[T] {
	n := len(l.s)
	l.b.used = int64(n + 1)
	l.s = l.s[:n+1]
	l.s[n] = x
	return l
}

// List_PushSlow is l with x appended (M_push).
func List_PushSlow[T any](l List[T], x T) List[T] {
	List_push(nil, &l, x)
	return l
}

// List_set is `xs.set(i, x)`: the fault is located at the caller's line.
func List_set[T any](_ *Task, l *List[T], i int64, x T) {
	if i < 0 || i >= int64(len(l.s)) {
		Faultf("", "index %d out of range for List of length %d", i, len(l.s))
	}
	List_SetAt(l, i, x, "")
}

// List_With returns a copy with element i replaced; l is unchanged.
func List_With[T any](l List[T], i int64, x T, at string) List[T] {
	List_markShared(l)
	List_SetAt(&l, i, x, at)
	return l
}

// List_push is `xs.push(x)` on a `var` list: amortized O(1). It appends in
// place when l ends at the block's claimed length (shared or not: no
// header sees that slot), else it copies.
func List_push[T any](_ *Task, l *List[T], x T) {
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
	List_copyOut(l, 1)
	l.b.used = int64(n + 1)
	l.s = l.s[:n+1]
	l.s[n] = x
}

// List_Push1 appends x in place when l is the unique owner and has room, and
// reports whether it did. Generated code pushes as
// `if !List_Push1(&xs, x) { List_push(tk, &xs, x) }`, which inlines the fast path.
func List_Push1[T any](l *List[T], x T) bool {
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

// List_pop is `xs.pop()`. The unique owner gives the slot back (the next
// push reuses it); a shared list only shortens its own view.
func List_pop[T any](_ *Task, l *List[T]) Option[T] {
	n := len(l.s)
	if n == 0 {
		return Option[T]{}
	}
	x := l.s[n-1]
	if List_unique(l) && l.b.used == int64(n) {
		var z T
		l.s[n-1] = z
		l.b.used = int64(n - 1)
	}
	l.s = l.s[:n-1]
	return Some(x)
}

// List_insert is `xs.insert(i, x)`; i may equal the length. In place for the
// unique owner (a shift, like a Go slice insert).
func List_insert[T any](_ *Task, l *List[T], i int64, x T) {
	n := len(l.s)
	if i < 0 || i > int64(n) {
		FaultHere("index %d out of range for insert into List of length %d", i, n)
	}
	List_own(l, 1)
	l.b.used = int64(n + 1)
	l.s = l.s[:n+1]
	copy(l.s[i+1:], l.s[i:n])
	l.s[i] = x
}

// List_remove_at is `xs.remove_at(i)`. In place for the unique owner (a
// shift, like a Go slice delete).
func List_remove_at[T any](_ *Task, l *List[T], i int64) T {
	x := List_At(*l, i, "")
	n := len(l.s)
	if !List_unique(l) {
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

// List_SliceRange is `xs[lo..hi]` with -1 for an omitted bound (core 3.6).
func List_SliceRange[T any](l List[T], lo, hi int64, hasLo, hasHi bool, at string) List[T] {
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
	List_markShared(l)
	return List[T]{s: l.s[lo:hi:hi], b: l.b}
}

// List_Concat is `xs + ys`. The result may share l's or o's array, so both
// are marked; o's elements are duplicated into the result, so they are
// marked too.
func List_Concat[T any](l List[T], o List[T]) List[T] {
	List_markShared(l)
	List_markShared(o)
	if len(o.s) == 0 {
		return l
	}
	if len(l.s) == 0 {
		return o
	}
	markElems(o.s)
	r := l
	for _, x := range o.s {
		List_push(nil, &r, x)
	}
	return r
}

// List_contains is `xs.contains(x)`.
func List_contains[T any](_ *Task, l List[T], x T) bool {
	for _, y := range l.s {
		if Eq(x, y) {
			return true
		}
	}
	return false
}

// List_index_of is `xs.index_of(x)`.
func List_index_of[T any](_ *Task, l List[T], x T) Option[int64] {
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

// List_reverse is `xs.reverse()`.
func List_reverse[T any](_ *Task, l List[T]) List[T] {
	out := make([]T, len(l.s))
	for i, x := range l.s {
		out[len(l.s)-1-i] = x
	}
	markElems(out)
	return ListOf(out...)
}

// List_sort is `xs.sort()`: stable, by the Ord of T.
func List_sort[T any](_ *Task, l List[T]) List[T] {
	out := slices.Clone(l.s)
	slices.SortStableFunc(out, func(a, b T) int { return Cmp(a, b) })
	markElems(out)
	return ListOf(out...)
}

// List_to_set is `xs.to_set()`.
func List_to_set[T any](_ *Task, l List[T]) Set[T] {
	var s Set[T]
	markElems(l.s)
	for _, x := range l.s {
		Set_add(nil, &s, x)
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

// List_filter_map is `xs.filter_map(f)`: f's Some values in order.
func List_filter_map[T, U, E any](tk *Task, l List[T], f func(*Task, T) Result[Option[U], E]) Result[List[U], E] {
	var out []U
	for _, x := range l.s {
		r := f(tk, x)
		if !r.Ok {
			return ErrTr[List[U]](r.E, r.Tr)
		}
		if r.V.Some {
			out = append(out, r.V.V)
		}
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
	// sort the positions by key, then copy the elements once: a scratch
	// array of (key, element) pairs copied every element twice
	keys := make([]K, len(l.s))
	for i, x := range l.s {
		r := key(tk, x)
		if !r.Ok {
			return ErrTr[List[T]](r.E, r.Tr)
		}
		keys[i] = r.V
	}
	perm := make([]int32, len(l.s))
	for i := range perm {
		perm[i] = int32(i)
	}
	slices.SortStableFunc(perm, func(a, b int32) int { return Cmp(keys[a], keys[b]) })
	out := make([]T, len(perm))
	for i, j := range perm {
		out[i] = l.s[j]
	}
	markElems(out)
	return Ok[List[T], E](ListOf(out...))
}

// The binary min heap in a list, container/heap's up and down by the Ord
// of T (decision 0410).
func heapUp[T any](s []T, j int) {
	for j > 0 {
		i := (j - 1) / 2
		if Cmp(s[j], s[i]) >= 0 {
			break
		}
		s[i], s[j] = s[j], s[i]
		j = i
	}
}

func heapDown[T any](s []T, i0, n int) bool {
	i := i0
	for {
		j1 := 2*i + 1
		if j1 >= n {
			break
		}
		j := j1
		if j2 := j1 + 1; j2 < n && Cmp(s[j2], s[j1]) < 0 {
			j = j2
		}
		if Cmp(s[j], s[i]) >= 0 {
			break
		}
		s[i], s[j] = s[j], s[i]
		i = j
	}
	return i > i0
}

// List_heap_push is `xs.heap_push(x)`.
func List_heap_push[T any](tk *Task, l *List[T], x T) {
	List_push(tk, l, x)
	List_own(l, 0)
	heapUp(l.s, len(l.s)-1)
	markElems(l.s)
}

// List_heap_pop is `xs.heap_pop()`.
func List_heap_pop[T any](tk *Task, l *List[T]) Option[T] {
	n := len(l.s) - 1
	if n < 0 {
		return Option[T]{}
	}
	List_own(l, 0)
	l.s[0], l.s[n] = l.s[n], l.s[0]
	heapDown(l.s, 0, n)
	markElems(l.s)
	return List_pop(tk, l)
}

// List_heap_remove is `xs.heap_remove(i)`.
func List_heap_remove[T any](tk *Task, l *List[T], i int64) T {
	n := len(l.s) - 1
	if i < 0 || i > int64(n) {
		FaultHere("List.heap_remove: index %d out of range for List of length %d", i, n+1)
	}
	List_own(l, 0)
	if n != int(i) {
		l.s[i], l.s[n] = l.s[n], l.s[i]
		if !heapDown(l.s, int(i), n) {
			heapUp(l.s, int(i))
		}
	}
	markElems(l.s)
	r := List_pop(tk, l)
	return r.V
}

// List_heap_fix is `xs.heap_fix(i)`.
func List_heap_fix[T any](_ *Task, l *List[T], i int64) {
	n := len(l.s)
	if i < 0 || i >= int64(n) {
		FaultHere("List.heap_fix: index %d out of range for List of length %d", i, n)
	}
	List_own(l, 0)
	if !heapDown(l.s, int(i), n) {
		heapUp(l.s, int(i))
	}
	markElems(l.s)
}

// List_heapify is `xs.heapify()`.
func List_heapify[T any](_ *Task, l *List[T]) {
	n := len(l.s)
	if n < 2 {
		return
	}
	List_own(l, 0)
	for i := n/2 - 1; i >= 0; i-- {
		heapDown(l.s, i, n)
	}
	markElems(l.s)
}

// List_sort_in_place is `xs.sort_in_place()` on a `var` list: stable, by
// the Ord of T, and it sorts the receiver (decision 0374). A list another
// binding shares is copied first, as a push copies one, so the other
// holder keeps its order; a uniquely owned list is sorted in its own array.
func List_sort_in_place[T any](_ *Task, l *List[T]) {
	if len(l.s) < 2 {
		return
	}
	List_own(l, 0)
	slices.SortStableFunc(l.s, func(a, b T) int { return Cmp(a, b) })
	markElems(l.s)
}

// List_sort_in_place_by is `xs.sort_in_place_by(key)`: stable, by the
// key's Ord, sorted in the receiver (the same copy-on-write first).
func List_sort_in_place_by[T, K, E any](tk *Task, l *List[T], key func(*Task, T) Result[K, E]) Result[Unit, E] {
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
	perm := make([]int32, len(l.s))
	for i := range perm {
		perm[i] = int32(i)
	}
	slices.SortStableFunc(perm, func(a, b int32) int { return Cmp(keys[a], keys[b]) })
	// put each element where its index says, then give the receiver the
	// fresh array and a block that describes it
	out := make([]T, len(l.s))
	for i, j := range perm {
		out[i] = l.s[j]
	}
	l.s, l.b = out, newBlock(len(out))
	markElems(l.s)
	return Ok[Unit, E](Unit{})
}

// List_group_by is `xs.group_by(key)`: the keys in first appearance order,
// each with its elements in input order.
func List_group_by[T, K, E any](tk *Task, l List[T], key func(*Task, T) Result[K, E]) Result[List[Tuple2[K, List[T]]], E] {
	var at Map[K, int64]
	var keys []K
	var groups [][]T
	for _, x := range l.s {
		r := key(tk, x)
		if !r.Ok {
			return ErrTr[List[Tuple2[K, List[T]]]](r.E, r.Tr)
		}
		if g := Map_get(tk, at, r.V); g.Some {
			groups[g.V] = append(groups[g.V], x)
		} else {
			Map_put(tk, &at, r.V, int64(len(keys)))
			keys = append(keys, r.V)
			groups = append(groups, []T{x})
		}
	}
	out := make([]Tuple2[K, List[T]], len(keys))
	for i, k := range keys {
		markElems(groups[i])
		out[i] = Tuple2[K, List[T]]{k, ListOf(groups[i]...)}
	}
	return Ok[List[Tuple2[K, List[T]]], E](ListOf(out...))
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

// List_with_capacity is `List[T].with_capacity(n)` (decision 0170): an
// empty list that owns a backing array with room for n elements, so the
// first n pushes append in place.
func List_with_capacity[T any](_ *Task, n int64) List[T] {
	return List[T]{s: make([]T, 0, max(n, 0)), b: newBlock(0)}
}

// M_push is `b.push(x)`.
func (b *Buf[T]) M_push(_ *Task, x T) { b.s = append(b.s, x) }

// M_len is `b.len()`.
func (b Buf[T]) M_len(*Task) int64 { return int64(len(b.s)) }

// Freeze is `b.freeze()`: moves the buffer into a list in O(1).
func (b Buf[T]) M_freeze(*Task) List[T] { return ListOf(b.s...) }

// All iterates the elements.
func (b Buf[T]) All() iter.Seq[T] { return List_All(List[T]{s: b.s}) }

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
	writeQuoted(sb, string(b.b))
	sb.WriteString(")")
}

// BytesBuilder is the move only byte string builder (core 5.3, decision
// 0078).
type BytesBuilder struct{ b []byte }

// BytesBuilder_new is `BytesBuilder.new()`.
func BytesBuilder_new(*Task) BytesBuilder { return BytesBuilder{} }

// BytesBuilder_with_capacity is `BytesBuilder.with_capacity(n)`: room for
// n bytes before the first growth (a negative n is no capacity).
func BytesBuilder_with_capacity(_ *Task, n int64) BytesBuilder {
	if n <= 0 {
		return BytesBuilder{}
	}
	return BytesBuilder{b: make([]byte, 0, n)}
}

// M_write is `b.write(data)`.
func (b *BytesBuilder) M_write(_ *Task, data Bytes) { b.b = append(b.b, data...) }

// M_write_byte is `b.write_byte(x)`.
func (b *BytesBuilder) M_write_byte(_ *Task, x uint8) { b.b = append(b.b, x) }

// M_write_str is `b.write_str(s)`.
func (b *BytesBuilder) M_write_str(_ *Task, s string) { b.b = append(b.b, s...) }

// M_write_rune is `b.write_rune(r)`: its UTF-8 encoding.
func (b *BytesBuilder) M_write_rune(_ *Task, r Rune) { b.b = utf8.AppendRune(b.b, rune(r)) }

// M_put_le is `b.put_le(at, v, n)`: the n (1, 2, 4 or 8) low bytes of v
// over b[at:at+n], lowest first, in place (decision 0399).
func (b *BytesBuilder) M_put_le(_ *Task, at, v, n int64) { b.put(at, v, n, false) }

// M_put_be is `b.put_be(at, v, n)`: highest byte first.
func (b *BytesBuilder) M_put_be(_ *Task, at, v, n int64) { b.put(at, v, n, true) }

func (b *BytesBuilder) put(at, v, n int64, be bool) {
	if (n != 1 && n != 2 && n != 4 && n != 8) || at < 0 || at > int64(len(b.b))-n {
		FaultHere("BytesBuilder.put: width must be 1, 2, 4 or 8 and at..at + width must lie within the builder")
	}
	p := b.b[at : at+n]
	for i := int64(0); i < n; i++ {
		x := byte(v >> (8 * uint(i)))
		if be {
			p[n-1-i] = x
		} else {
			p[i] = x
		}
	}
}

// M_len is `b.len()`.
func (b BytesBuilder) M_len(*Task) int64 { return int64(len(b.b)) }

// M_freeze is `b.freeze()`.
func (b BytesBuilder) M_freeze(*Task) Bytes { return Bytes(b.b) }

// XoDebug implements Debug.
func (b BytesBuilder) XoDebug(sb *strings.Builder) {
	sb.WriteString("BytesBuilder(b")
	writeQuoted(sb, string(b.b))
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

// List_Items returns the elements of a list for iteration; callers must not
// modify them.
func List_Items[T any](l List[T]) []T { return l.s }
