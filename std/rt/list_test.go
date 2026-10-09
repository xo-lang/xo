package rt

import (
	"math/rand/v2"
	"slices"
	"sync"
	"testing"
)

// The tests drive List the way generated code does (internal/gen/own.go):
// a copy of a value out of a place goes through Own, an element write
// through Mine/Unshared/PutM, a nested write through PtrM, a push through
// CanPush/PushFast/PushSlow.

func setL[T any](l *List[T], i int, x T) {
	if !List_Mine(*l) {
		*l = List_Unshared(*l)
	}
	List_PutM(*l, int64(i), x, "test")
}

func pushL[T any](l *List[T], x T) {
	if List_CanPush(*l) {
		*l = List_PushFast(*l, x)
	} else {
		*l = List_PushSlow(*l, x)
	}
}

func checkList(t *testing.T, where string, l List[int64], want []int64) {
	t.Helper()
	if !slices.Equal(List_Slice(l), want) {
		t.Fatalf("%s: %v, want %v", where, List_Slice(l), want)
	}
}

// TestListModel runs random writes, pushes, pops, inserts, removals,
// copies, slices, and concatenations over a pool of list values and
// checks every value against a deep copied reference after each step.
func TestListModel(t *testing.T) {
	for seed := range uint64(60) {
		rng := rand.New(rand.NewPCG(seed, 11))
		pool := make([]List[int64], 5)
		refs := make([][]int64, 5)
		for step := range 4000 {
			i := rng.IntN(len(pool))
			l, r := &pool[i], refs[i]
			v := int64(rng.IntN(1000))
			switch op := rng.IntN(11); {
			case op == 0: // copy out of a place
				j := rng.IntN(len(pool))
				pool[j] = List_Own(pool[i])
				refs[j] = slices.Clone(r)
			case op <= 2 && len(r) > 0:
				k := rng.IntN(len(r))
				setL(l, k, v)
				r[k] = v
			case op <= 4:
				pushL(l, v)
				refs[i] = append(r, v)
			case op == 5 && len(r) > 0:
				got := List_pop(nil, l)
				if !got.Some || got.V != r[len(r)-1] {
					t.Fatalf("seed %d step %d: pop %v, want %d", seed, step, got, r[len(r)-1])
				}
				refs[i] = r[:len(r)-1]
			case op == 6:
				k := rng.IntN(len(r) + 1)
				List_insert(nil, l, int64(k), v)
				refs[i] = slices.Insert(r, k, v)
			case op == 7 && len(r) > 0:
				k := rng.IntN(len(r))
				if got := List_remove_at(nil, l, int64(k)); got != r[k] {
					t.Fatalf("seed %d step %d: remove_at %d, want %d", seed, step, got, r[k])
				}
				refs[i] = slices.Delete(r, k, k+1)
			case op == 8:
				lo := rng.IntN(len(r) + 1)
				hi := lo + rng.IntN(len(r)-lo+1)
				j := rng.IntN(len(pool))
				pool[j] = List_SliceRange(pool[i], int64(lo), int64(hi), true, true, "test")
				refs[j] = slices.Clone(r[lo:hi])
			case op == 9:
				j := rng.IntN(len(pool))
				k := rng.IntN(len(pool))
				pool[k] = List_Concat(pool[i], pool[j])
				refs[k] = append(slices.Clone(refs[i]), refs[j]...)
			case op == 10 && len(r) > 0:
				k := rng.IntN(len(r))
				List_set(nil, l, int64(k), v)
				r[k] = v
			}
			for k := range pool {
				checkList(t, "model", pool[k], refs[k])
			}
		}
	}
}

// TestListNested writes rows of a list of lists through PtrM after
// copying the outer list, reading a row out, and pushing rows: the lazy
// marking of inner lists (markElems on a copy) must keep every copy
// intact.
func TestListNested(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 5))
	type ref = [][]int64
	clone := func(g ref) ref {
		out := make(ref, len(g))
		for i, r := range g {
			out[i] = slices.Clone(r)
		}
		return out
	}
	var pool [3]List[List[int64]]
	var refs [3]ref
	var rows []List[int64]
	var rowRefs [][]int64
	for step := range 5000 {
		i := rng.IntN(3)
		g, r := &pool[i], refs[i]
		v := int64(rng.IntN(100))
		switch op := rng.IntN(6); {
		case op == 0:
			j := rng.IntN(3)
			pool[j] = List_Own(pool[i])
			refs[j] = clone(r)
		case op == 1 && len(r) > 0:
			// `let row = g[k]`: an element copied out of a place
			k := rng.IntN(len(r))
			rows = append(rows, List_Own(List_AtM(*g, int64(k), "test")))
			rowRefs = append(rowRefs, slices.Clone(r[k]))
		case op == 2 && len(r) > 0 && len(r[0]) > 0:
			// `g[k][c] = v`
			k := rng.IntN(len(r))
			if len(r[k]) == 0 {
				break
			}
			c := rng.IntN(len(r[k]))
			if !List_Mine(*g) {
				*g = List_Unshared(*g)
			}
			setL(List_PtrM(*g, int64(k), "test"), c, v)
			r[k][c] = v
		case op == 3 && len(r) > 0:
			// `g[k].push(v)` through the element's address
			k := rng.IntN(len(r))
			if !List_Mine(*g) {
				*g = List_Unshared(*g)
			}
			pushL(List_PtrM(*g, int64(k), "test"), v)
			r[k] = append(r[k], v)
		case op == 4:
			// `g.push(row)` of a fresh row
			pushL(g, ListOf(v, v+1))
			refs[i] = append(r, []int64{v, v + 1})
		case op == 5 && len(rows) > 0:
			// `g.push(row)` of a row read out earlier (an own site)
			k := rng.IntN(len(rows))
			pushL(g, List_Own(rows[k]))
			refs[i] = append(r, slices.Clone(rowRefs[k]))
		}
		for k := range pool {
			got := List_Slice(pool[k])
			if len(got) != len(refs[k]) {
				t.Fatalf("step %d: list %d has %d rows, want %d", step, k, len(got), len(refs[k]))
			}
			for j := range got {
				checkList(t, "nested", got[j], refs[k][j])
			}
		}
		for k := range rows {
			checkList(t, "row", rows[k], rowRefs[k])
		}
	}
}

// TestListConcurrentReaders hands marked snapshots to other goroutines
// while the owner keeps writing and pushing its own value: the readers
// must always see their snapshot, without data races (-race).
func TestListConcurrentReaders(t *testing.T) {
	var l List[int64]
	for i := range int64(64) {
		pushL(&l, i)
	}
	var wg sync.WaitGroup
	for round := range 8 {
		snap := List_Own(l)
		want := slices.Clone(List_Slice(snap))
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 200 {
				if !slices.Equal(List_Slice(snap), want) {
					t.Errorf("round %d: snapshot changed", round)
					return
				}
			}
			// the reader writes its own copy too
			c := List_Own(snap)
			setL(&c, 0, -1)
			pushL(&c, -2)
		}()
		for i := range 64 {
			setL(&l, i, int64(round*100+i))
		}
		pushL(&l, int64(round))
	}
	wg.Wait()
}

func BenchmarkListSetAt(b *testing.B) {
	var l List[int64]
	for i := range int64(50000) {
		pushL(&l, i)
	}
	b.ResetTimer()
	for i := range b.N {
		k := i % 50000
		setL(&l, k, int64(i))
	}
}

func BenchmarkListPushPop(b *testing.B) {
	var l List[int64]
	for i := range b.N {
		pushL(&l, int64(i))
		if i%3 == 2 {
			List_pop(nil, &l)
			List_pop(nil, &l)
		}
	}
}

func BenchmarkListPush(b *testing.B) {
	for range b.N {
		var l List[int64]
		for i := range int64(1000) {
			pushL(&l, i)
		}
	}
}

func BenchmarkListCopySet(b *testing.B) {
	var base List[int64]
	for i := range int64(50000) {
		pushL(&base, i)
	}
	b.ResetTimer()
	for i := range b.N {
		c := List_Own(base)
		setL(&c, i%50000, -1)
	}
}

func BenchmarkListInsertMiddle(b *testing.B) {
	for range b.N {
		var l List[int64]
		for i := range int64(2000) {
			List_insert(nil, &l, int64(List_len(nil, l)/2), i)
		}
	}
}
