package rt

import (
	"fmt"
	"iter"
	"strings"
	"sync/atomic"
)

// Map is the insertion ordered map `Map[K, V]` with value semantics (core
// 2.1, 5.1, 5.2; std.md 2.4).
//
// # Representation
//
// A Map value is a small header (t, es, n): a table t, the prefix es of
// t's entry log that this value sees, and the number n of live keys. A
// table is an append only log of entries plus a hash index from each key
// to its newest entry. A put appends an entry (key, value, the previous
// entry of the same key, and the origin: the log position that gives the
// key its place in insertion order); a remove appends a tombstone. The
// value of a header is a pure function of its log prefix: the value of k
// is the newest entry of k below the header's length L (absent if there is
// none or it is a tombstone), and the order of the keys is the order of
// their origins.
//
// Ownership is claimed the way List claims its backing array: t.used is
// the log length some value has claimed. A put through a header whose
// length L equals t.used claims L+1 with a compare and swap and appends in
// place (amortized O(1), no copying). A header with L < t.used is stale:
// another value already extended the same log. A stale put forks: it makes
// a new table whose parent is the stale header's view (t, L), so the fork
// costs O(1) and its own entries live in its own log and index. Lookups in
// a fork search its own index, then the parent view. Forks nest at most
// maxForkDepth deep; deeper, or when a log holds more garbage (overwritten
// entries and tombstones) than live keys, the view is flattened into a
// fresh table in O(n), which amortizes against the appends that made the
// garbage.
//
// # Soundness (value semantics)
//
// Claim: no operation on one Map value changes the value of another, no
// matter how headers were copied (Go assignment, struct copy, closure
// capture, channel send, storing in a collection).
//
//  1. The value of a header (t, es, n) depends only on entries
//     [t.base, t.base+len(es)) of t and, recursively, on the parent view
//     (t.parent, t.base). Entries are never written after they are
//     appended and the parent view is fixed when the fork is made.
//  2. The only writes to a table are appends at position t.used (after
//     claiming it), index slot updates, index growth, and log array growth.
//     The claimed position is at or past the end of every header's view, so
//     an append never changes an entry some header sees.
//  3. The index is not part of any view: a lookup from a view of length L
//     takes the newest entry of k the index names and follows the prev
//     links (each entry links to the previous entry of the same key in the
//     same table, so the chain lists every entry of k in decreasing
//     order) until it is below L. That gives the newest entry of k below L
//     whatever the index currently says, so updating the index for later
//     entries does not change the result.
//  4. Two headers may share a length L (copies of one value). Only one of
//     them wins the compare and swap from L to L+1; the other sees
//     t.used != L and forks, so at most one value ever appends at a given
//     position and each position has exactly one entry.
//
// Hence a header's value is fixed from its creation until it is
// overwritten, which is value semantics. Copies cost O(1); the first put
// after a copy decides which of the two copies keeps appending in place.
//
// # Concurrency
//
// Copies of a map can be read on other tasks (goroutines) while the owner
// appends. Index slots, the index pointer, the log array pointer, and
// t.used are atomics. The owner writes an entry, then publishes it (array
// pointer, then the slot store); a reader that gets an entry position from
// a slot load sees the entry. Entries below a reader's own length were
// written before the reader's header existed, so they are ordered before
// its reads by whatever handed the header over (a channel, a mutex, task
// spawn). Table metadata that is not atomic (t.keys) is touched only by the
// owner after a successful claim; the previous owner's writes happen before
// that claim because the new owner's header was copied from it after its
// put returned.
type Map[K, V any] struct {
	t  *mtable[K, V]
	es []mentry[K, V] // t's own entries visible to this value
	n  int
}

// mentry is one log entry. Positions are logical: a fork's first own entry
// has position t.base. Tombstones have org < 0 and a zero value.
type mentry[K, V any] struct {
	h    uint64
	prev int32 // position of the previous entry of k in the same table, or -1
	org  int32 // position that gives k its place in insertion order, or -1
	k    K
	v    V
}

type mtable[K, V any] struct {
	parent *mtable[K, V] // the view this table forked from (with base), or nil
	base   int
	depth  int
	used   atomic.Int64                   // log length claimed by some value
	arr    atomic.Pointer[[]mentry[K, V]] // newest log array (len == cap)
	idx    atomic.Pointer[mindex]         // own keys to newest own entry
	keys   int                            // distinct keys in idx (owner only)
}

// mindex is an open addressing index: a slot holds an own entry position
// minus base plus one, or 0 when empty.
type mindex struct {
	slots []atomic.Int32
	shift uint
}

const maxForkDepth = 8

func newIndex(keys int) *mindex {
	b := uint(3)
	for 1<<b < 2*keys {
		b++
	}
	return &mindex{slots: make([]atomic.Int32, 1<<b), shift: 64 - b}
}

func (ix *mindex) home(h uint64) uint64 { return (h * 0x9e3779b97f4a7c15) >> ix.shift }

// at is the entry at position j of t; es is the viewing header's own
// entries when t is its table (nil for a parent level).
func (t *mtable[K, V]) at(es []mentry[K, V], j int) *mentry[K, V] {
	i := j - t.base
	if i < len(es) {
		return &es[i]
	}
	return &(*t.arr.Load())[i]
}

// probe returns the slot of k in ix and the position of k's newest own
// entry, or the empty slot where k would go and -1.
func (t *mtable[K, V]) probe(es []mentry[K, V], ix *mindex, h uint64, k K) (uint64, int) {
	mask := uint64(len(ix.slots) - 1)
	for p := ix.home(h); ; p = (p + 1) & mask {
		v := ix.slots[p].Load()
		if v == 0 {
			return p, -1
		}
		j := t.base + int(v) - 1
		if e := t.at(es, j); e.h == h && Eq(e.k, k) {
			return p, j
		}
	}
}

// mfind is the newest live entry of k in the view (t, es) of length L, or
// nil. It follows prev links down to the view and falls back to parents.
func mfind[K, V any](t *mtable[K, V], es []mentry[K, V], L int, h uint64, k K) *mentry[K, V] {
	for t != nil {
		if L > t.base {
			_, j := t.probe(es, t.idx.Load(), h, k)
			for j >= L {
				j = int(t.at(es, j).prev)
			}
			if j >= 0 {
				if e := t.at(es, j); e.org >= 0 {
					return e
				}
				return nil
			}
		}
		L, t, es = t.base, t.parent, nil
	}
	return nil
}

func (m *Map[K, V]) find(h uint64, k K) *mentry[K, V] {
	if m.t == nil {
		return nil
	}
	return mfind(m.t, m.es, m.t.base+len(m.es), h, k)
}

// claim makes m the owner of the next log position of its table: it wins
// the compare and swap, or forks (or flattens) into a table only m has.
func (m *Map[K, V]) claim() {
	t := m.t
	if t == nil {
		m.t, m.es = &mtable[K, V]{}, nil
		m.t.used.Store(1)
		return
	}
	L := int64(t.base + len(m.es))
	if L >= 1<<31-1 {
		FaultHere("Map log longer than %d entries", L)
	}
	if t.used.CompareAndSwap(L, L+1) {
		return
	}
	if t.depth >= maxForkDepth {
		m.flatten(8)
	} else {
		m.t, m.es = &mtable[K, V]{parent: t, base: int(L), depth: t.depth + 1}, nil
	}
	m.t.used.Store(int64(m.t.base+len(m.es)) + 1)
}

// record appends an entry for k (a tombstone when del) at the position m
// claimed. Only the owner calls it.
func (m *Map[K, V]) record(h uint64, k K, v V, del bool) {
	t := m.t
	ix := t.idx.Load()
	if ix == nil {
		ix = newIndex(4)
		t.idx.Store(ix)
	}
	pos, j := t.probe(m.es, ix, h, k)
	m.write(ix, pos, j, h, k, v, del)
}

// write is record after the probe: (pos, j) is the result of probing ix,
// t's current index, for k.
func (m *Map[K, V]) write(ix *mindex, pos uint64, j int, h uint64, k K, v V, del bool) {
	t := m.t
	L := t.base + len(m.es)
	var org int32
	switch {
	case j >= 0:
		org = t.at(m.es, j).org
	case t.parent != nil:
		if e := mfind(t.parent, nil, t.base, h, k); e != nil {
			org = e.org
		} else {
			org = -1
		}
	default:
		org = -1
	}
	if j < 0 {
		if 2*(t.keys+1) > len(ix.slots) {
			ix = t.grow(m.es, ix)
			pos, _ = t.probe(m.es, ix, h, k)
		}
		t.keys++
	}
	switch {
	case del:
		m.n--
		org = -1
		var zero V
		v = zero
	case org < 0:
		m.n++
		org = int32(L)
	}
	e := mentry[K, V]{h: h, prev: int32(j), org: org, k: k, v: v}
	i := len(m.es)
	if i < cap(m.es) {
		m.es = m.es[:i+1]
		m.es[i] = e
	} else {
		a := make([]mentry[K, V], max(8, 2*cap(m.es)))
		copy(a, m.es)
		a[i] = e
		t.arr.Store(&a)
		m.es = a[:i+1]
	}
	ix.slots[pos].Store(int32(i + 1))
	if t.parent == nil && L+1 > 2*m.n+32 || t.parent != nil && i+1 > m.n+32 {
		m.flatten(m.n + 32)
	}
}

// grow doubles the index, publishing the new one; old readers keep the old
// index, which still names every key of their views.
func (t *mtable[K, V]) grow(es []mentry[K, V], ix *mindex) *mindex {
	nx := newIndex(len(ix.slots))
	mask := uint64(len(nx.slots) - 1)
	for i := range ix.slots {
		v := ix.slots[i].Load()
		if v == 0 {
			continue
		}
		p := nx.home(es[v-1].h)
		for nx.slots[p].Load() != 0 {
			p = (p + 1) & mask
		}
		nx.slots[p].Store(v)
	}
	t.idx.Store(nx)
	return nx
}

// flatten replaces m's table with a fresh one holding only its live
// entries, with room for extra more.
func (m *Map[K, V]) flatten(extra int) {
	if m.t.parent == nil && int64(len(m.es)) == m.t.used.Load() {
		m.flattenOwned(extra)
		return
	}
	t := &mtable[K, V]{}
	a := make([]mentry[K, V], m.n+extra+8)
	ix := newIndex(m.n + 1)
	mask := uint64(len(ix.slots) - 1)
	i := 0
	m.each(func(e *mentry[K, V], v V) bool {
		a[i] = mentry[K, V]{h: e.h, prev: -1, org: int32(i), k: e.k, v: v}
		p := ix.home(e.h)
		for ix.slots[p].Load() != 0 {
			p = (p + 1) & mask
		}
		ix.slots[p].Store(int32(i + 1))
		i++
		return true
	})
	t.keys = i
	t.arr.Store(&a)
	t.idx.Store(ix)
	t.used.Store(int64(i))
	m.t, m.es = t, a[:i]
}

// flattenOwned is flatten for the owner of a root table: the index names
// the newest entry of every key, so no lookups are needed; live entries
// are placed by their origin.
func (m *Map[K, V]) flattenOwned(extra int) {
	ix := m.t.idx.Load()
	byOrg := make([]int32, len(m.es))
	for i := range ix.slots {
		if v := ix.slots[i].Load(); v != 0 {
			if e := &m.es[v-1]; e.org >= 0 {
				byOrg[e.org] = v
			}
		}
	}
	t := &mtable[K, V]{}
	a := make([]mentry[K, V], m.n+extra+8)
	nx := newIndex(m.n + 1)
	mask := uint64(len(nx.slots) - 1)
	i := 0
	for _, v := range byOrg {
		if v == 0 {
			continue
		}
		e := &m.es[v-1]
		a[i] = mentry[K, V]{h: e.h, prev: -1, org: int32(i), k: e.k, v: e.v}
		p := nx.home(e.h)
		for nx.slots[p].Load() != 0 {
			p = (p + 1) & mask
		}
		nx.slots[p].Store(int32(i + 1))
		i++
	}
	t.keys = i
	t.arr.Store(&a)
	t.idx.Store(nx)
	t.used.Store(int64(i))
	m.t, m.es = t, a[:i]
}

// each calls f with the origin entry and current value of every live key,
// in insertion order, until f returns false.
func (m *Map[K, V]) each(f func(e *mentry[K, V], v V) bool) {
	t := m.t
	if t == nil {
		return
	}
	L := t.base + len(m.es)
	if t.parent == nil && L == m.n {
		// No overwrites or removals in view: every entry is live.
		for i := range m.es {
			if !f(&m.es[i], m.es[i].v) {
				return
			}
		}
		return
	}
	var levels [maxForkDepth + 1][]mentry[K, V]
	var bases [maxForkDepth + 1]int
	d := 0
	end := L
	for x, es := t, m.es; x != nil; x, es = x.parent, nil {
		if es == nil && end > x.base {
			es = (*x.arr.Load())[:end-x.base]
		}
		levels[d], bases[d] = es, x.base
		end = x.base
		d++
	}
	for d--; d >= 0; d-- {
		for i := range levels[d] {
			e := &levels[d][i]
			if int(e.org) != bases[d]+i {
				continue
			}
			if cur := mfind(t, m.es, L, e.h, e.k); cur != nil && cur.org == e.org {
				if !f(e, cur.v) {
					return
				}
			}
		}
	}
}

// MapOf builds a map from alternating keys and values in a literal.
func MapOf[K, V any](entries ...Tuple2[K, V]) Map[K, V] {
	var m Map[K, V]
	for _, e := range entries {
		m.M_put(nil, e.F0, e.F1)
	}
	return m
}

// M_len is `m.len()`.
func (m Map[K, V]) M_len(*Task) int64 { return int64(m.n) }

// M_is_empty is `m.is_empty()`.
func (m Map[K, V]) M_is_empty(*Task) bool { return m.n == 0 }

// M_get is `m.get(k)`.
func (m Map[K, V]) M_get(_ *Task, k K) Option[V] {
	if e := m.find(Hash(k), k); e != nil {
		return Some(e.v)
	}
	return Option[V]{}
}

// At is `m[k]`: faults when the key is missing (core 3.6).
func (m Map[K, V]) At(k K, at string) V {
	if e := m.find(Hash(k), k); e != nil {
		return e.v
	}
	Faultf(at, "key %s not found in Map of length %d", Debug(k), m.n)
	panic("unreachable")
}

// M_get_or_fault is `m.get_or_fault(k, reason)`: the value, or a fault naming
// the key and the reason it was expected to exist (core 3.6).
func (m Map[K, V]) M_get_or_fault(_ *Task, k K, reason string) V {
	if e := m.find(Hash(k), k); e != nil {
		return e.v
	}
	orFault(fmt.Sprintf("key %s not found in Map of length %d: %s", Debug(k), m.n, reason))
	panic("unreachable")
}

// M_contains is `m.contains(k)`.
func (m Map[K, V]) M_contains(_ *Task, k K) bool { return m.find(Hash(k), k) != nil }

// M_put is `m.put(k, v)` and `m[k] = v` on a `var` map.
func (m *Map[K, V]) M_put(_ *Task, k K, v V) {
	h := Hash(k)
	m.claim()
	m.record(h, k, v, false)
}

// With returns a copy with k set to v.
func (m Map[K, V]) With(k K, v V) Map[K, V] {
	m.M_put(nil, k, v)
	return m
}

// M_remove is `m.remove(k)`.
func (m *Map[K, V]) M_remove(_ *Task, k K) Option[V] {
	h := Hash(k)
	e := m.find(h, k)
	if e == nil {
		return Option[V]{}
	}
	v := e.v
	m.claim()
	m.record(h, k, v, true)
	return Some(v)
}

// All iterates the entries in insertion order.
func (m Map[K, V]) All() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		m.each(func(e *mentry[K, V], v V) bool { return yield(e.k, v) })
	}
}

// Pairs iterates the entries as tuples (for `for (k, v) in m`).
func (m Map[K, V]) Pairs() iter.Seq[Tuple2[K, V]] {
	return func(yield func(Tuple2[K, V]) bool) {
		for k, v := range m.All() {
			if !yield(Tuple2[K, V]{k, v}) {
				return
			}
		}
	}
}

// M_keys is `m.keys()`.
func (m Map[K, V]) M_keys(*Task) List[K] {
	out := make([]K, 0, m.n)
	for k := range m.All() {
		out = append(out, k)
	}
	// the elements stay in the map too (see List)
	markElems(out)
	return ListOf(out...)
}

// M_values is `m.values()`.
func (m Map[K, V]) M_values(*Task) List[V] {
	out := make([]V, 0, m.n)
	for _, v := range m.All() {
		out = append(out, v)
	}
	// the elements stay in the map too (see List)
	markElems(out)
	return ListOf(out...)
}

// Map_entries is `m.entries()`.
func Map_entries[K, V any](_ *Task, m Map[K, V]) List[Tuple2[K, V]] {
	out := make([]Tuple2[K, V], 0, m.n)
	for k, v := range m.All() {
		out = append(out, Tuple2[K, V]{k, v})
	}
	// the elements stay in the map too (see List)
	markElems(out)
	return ListOf(out...)
}

// Map_filter is `m.filter(f)`.
func Map_filter[K, V, E any](tk *Task, m Map[K, V], f func(*Task, K, V) Result[bool, E]) Result[Map[K, V], E] {
	var out Map[K, V]
	for k, v := range m.All() {
		r := f(tk, k, v)
		if !r.Ok {
			return ErrTr[Map[K, V]](r.E, r.Tr)
		}
		if r.V {
			out.M_put(tk, k, v)
		}
	}
	return Ok[Map[K, V], E](out)
}

// Map_update is `m.update(k, init, f)`: reads k (or init), runs f on a
// mutable copy, stores the result, and returns f's value (std.md 2.4). If f
// fails the map is unchanged.
//
// The fast path probes once and reuses the probe for the write. That is
// sound because the write only happens after winning the claim of the
// position the probe saw as the end of the log: no value can have written
// the index in between (point 4 of the Map comment), and m itself is
// checked to be unchanged.
func Map_update[K, V, R, E any](tk *Task, m *Map[K, V], k K, init V, f func(*Task, *V) Result[R, E]) Result[R, E] {
	h := Hash(k)
	t := m.t
	if t != nil && t.parent == nil {
		// Fast path for the owner of a root table: one probe serves the
		// read and the write. Nothing else can write t's index until
		// someone claims position L, so the probe stays valid if the
		// claim below succeeds.
		L := len(m.es)
		if ix := t.idx.Load(); ix != nil && t.used.Load() == int64(L) {
			pos, j := t.probe(m.es, ix, h, k)
			v := init
			if j >= 0 && m.es[j].org >= 0 {
				v = m.es[j].v
			}
			// v is still in the log: writes through f must copy its lists
			MarkP(&v)
			r := f(tk, &v)
			if r.Ok {
				if m.t == t && len(m.es) == L && t.idx.Load() == ix && t.used.CompareAndSwap(int64(L), int64(L+1)) {
					m.write(ix, pos, j, h, k, v, false)
				} else {
					m.claim()
					m.record(h, k, v, false)
				}
			}
			return r
		}
	}
	v := init
	if s := m.find(h, k); s != nil {
		v = s.v
	}
	MarkP(&v)
	r := f(tk, &v)
	if r.Ok {
		m.claim()
		m.record(h, k, v, false)
	}
	return r
}

// XoEq implements Eq: the same entries, in any order.
func (m Map[K, V]) XoEq(o any) bool {
	p, ok := o.(Map[K, V])
	if !ok || m.n != p.n {
		return false
	}
	for k, v := range m.All() {
		s := p.find(Hash(k), k)
		if s == nil || !Eq(v, s.v) {
			return false
		}
	}
	return true
}

// XoHash implements Hash, independent of order.
func (m Map[K, V]) XoHash() uint64 {
	h := uint64(0x6d6170)
	for k, v := range m.All() {
		h += mix(Hash(k), Hash(v))
	}
	return h
}

// XoDebug implements Debug: `{"a": 1}`.
func (m Map[K, V]) XoDebug(b *strings.Builder) {
	b.WriteByte('{')
	i := 0
	for k, v := range m.All() {
		if i > 0 {
			b.WriteString(", ")
		}
		debugTo(b, k)
		b.WriteString(": ")
		debugTo(b, v)
		i++
	}
	b.WriteByte('}')
}

// Set is the insertion ordered set `Set[T]`.
type Set[T any] struct {
	m Map[T, Unit]
}

// SetOf builds a set from elements.
func SetOf[T any](xs ...T) Set[T] {
	var s Set[T]
	for _, x := range xs {
		s.M_add(nil, x)
	}
	return s
}

// Len is `s.len()`.
func (s Set[T]) M_len(tk *Task) int64 { return s.m.M_len(tk) }

// IsEmpty is `s.is_empty()`.
func (s Set[T]) M_is_empty(*Task) bool { return s.m.n == 0 }

// Contains is `s.contains(x)`.
func (s Set[T]) M_contains(tk *Task, x T) bool { return s.m.M_contains(tk, x) }

// Add is `s.add(x)`.
func (s *Set[T]) M_add(_ *Task, x T) {
	if !s.m.M_contains(nil, x) {
		s.m.M_put(nil, x, Unit{})
	}
}

// Remove is `s.remove(x)`.
func (s *Set[T]) M_remove(tk *Task, x T) bool { return s.m.M_remove(tk, x).Some }

// All iterates the elements in insertion order.
func (s Set[T]) All() iter.Seq[T] {
	return func(yield func(T) bool) {
		for k := range s.m.All() {
			if !yield(k) {
				return
			}
		}
	}
}

// ToList is `s.to_list()`.
func (s Set[T]) M_to_list(tk *Task) List[T] { return s.m.M_keys(tk) }

// Union is `s.union(o)`.
func (s Set[T]) M_union(_ *Task, o Set[T]) Set[T] {
	r := s
	for x := range o.All() {
		r.M_add(nil, x)
	}
	return r
}

// Intersect is `s.intersect(o)`.
func (s Set[T]) M_intersect(_ *Task, o Set[T]) Set[T] {
	var r Set[T]
	for x := range s.All() {
		if o.M_contains(nil, x) {
			r.M_add(nil, x)
		}
	}
	return r
}

// Difference is `s.difference(o)`.
func (s Set[T]) M_difference(_ *Task, o Set[T]) Set[T] {
	var r Set[T]
	for x := range s.All() {
		if !o.M_contains(nil, x) {
			r.M_add(nil, x)
		}
	}
	return r
}

// SetAny is `s.any(f)`.
func Set_any[T, E any](tk *Task, s Set[T], f func(*Task, T) Result[bool, E]) Result[bool, E] {
	return List_any(tk, s.M_to_list(tk), f)
}

// SetAll is `s.all(f)`.
func Set_all[T, E any](tk *Task, s Set[T], f func(*Task, T) Result[bool, E]) Result[bool, E] {
	return List_all(tk, s.M_to_list(tk), f)
}

// SetFilter is `s.filter(f)`.
func Set_filter[T, E any](tk *Task, s Set[T], f func(*Task, T) Result[bool, E]) Result[Set[T], E] {
	r := List_filter(tk, s.M_to_list(tk), f)
	if !r.Ok {
		return ErrTr[Set[T]](r.E, r.Tr)
	}
	return Ok[Set[T], E](r.V.M_to_set(tk))
}

// SetMap is `s.map(f)`.
func Set_map[T, U, E any](tk *Task, s Set[T], f func(*Task, T) Result[U, E]) Result[Set[U], E] {
	r := List_map(tk, s.M_to_list(tk), f)
	if !r.Ok {
		return ErrTr[Set[U]](r.E, r.Tr)
	}
	return Ok[Set[U], E](r.V.M_to_set(tk))
}

// XoEq implements Eq: the same elements, in any order.
func (s Set[T]) XoEq(o any) bool {
	p, ok := o.(Set[T])
	return ok && s.m.XoEq(p.m)
}

// XoHash implements Hash.
func (s Set[T]) XoHash() uint64 { return s.m.XoHash() ^ 0x736574 }

// XoDebug implements Debug: `#{1, 2}`.
func (s Set[T]) XoDebug(b *strings.Builder) {
	b.WriteString("#{")
	i := 0
	for x := range s.All() {
		if i > 0 {
			b.WriteString(", ")
		}
		debugTo(b, x)
		i++
	}
	b.WriteByte('}')
}

// Range is the value of `a..b` or `a..=b` (core 3.5).
type Range[T Integer] struct {
	Lo, Hi T
	Incl   bool
}

// All iterates the range.
func (r Range[T]) All() iter.Seq[T] {
	return func(yield func(T) bool) {
		if r.Incl {
			if r.Lo > r.Hi {
				return
			}
			for i := r.Lo; ; i++ {
				if !yield(i) || i == r.Hi {
					return
				}
			}
		}
		for i := r.Lo; i < r.Hi; i++ {
			if !yield(i) {
				return
			}
		}
	}
}

// Incl iterates `lo..=hi` without overflowing at the maximum value.
func Incl[T Integer](lo, hi T) iter.Seq[T] { return Range[T]{lo, hi, true}.All() }

// XoDebug implements Debug.
func (r Range[T]) XoDebug(b *strings.Builder) {
	debugTo(b, r.Lo)
	if r.Incl {
		b.WriteString("..=")
	} else {
		b.WriteString("..")
	}
	debugTo(b, r.Hi)
}
