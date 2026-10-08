package rt

import (
	"math/rand/v2"
	"sync"
	"testing"
)

// mmodel is the reference for a Map value: a Go map plus insertion order.
type mmodel struct {
	vals  map[int64]int64
	order []int64
}

func (r mmodel) clone() mmodel {
	c := mmodel{vals: make(map[int64]int64, len(r.vals)), order: append([]int64(nil), r.order...)}
	for k, v := range r.vals {
		c.vals[k] = v
	}
	return c
}

func (r *mmodel) put(k, v int64) {
	if _, ok := r.vals[k]; !ok {
		r.order = append(r.order, k)
	}
	r.vals[k] = v
}

func (r *mmodel) remove(k int64) {
	if _, ok := r.vals[k]; !ok {
		return
	}
	delete(r.vals, k)
	for i, x := range r.order {
		if x == k {
			r.order = append(r.order[:i:i], r.order[i+1:]...)
			break
		}
	}
}

func checkModel(t *testing.T, where string, m Map[int64, int64], r mmodel, keys int64) {
	t.Helper()
	if int(m.M_len(nil)) != len(r.vals) {
		t.Fatalf("%s: len %d, want %d", where, m.M_len(nil), len(r.vals))
	}
	i := 0
	for k, v := range m.All() {
		if i >= len(r.order) || r.order[i] != k || r.vals[k] != v {
			t.Fatalf("%s: entry %d is %d=%d, want order %v", where, i, k, v, r.order)
		}
		i++
	}
	if i != len(r.order) {
		t.Fatalf("%s: iterated %d entries, want %d", where, i, len(r.order))
	}
	for k := range keys {
		got := m.M_get(nil, k)
		want, ok := r.vals[k]
		if got.Some != ok || got.V != want {
			t.Fatalf("%s: get(%d) = %v, want %d %v", where, k, got, want, ok)
		}
	}
}

// TestMapModel runs random puts, removes, and copies over a pool of map
// values and checks every value against its reference after each step: a
// change through one value must never show in another (value semantics),
// whichever of two copies writes first, through forks and flattening.
func TestMapModel(t *testing.T) {
	for seed := range uint64(40) {
		rng := rand.New(rand.NewPCG(seed, 7))
		keys := int64(1 + rng.IntN(60))
		pool := make([]Map[int64, int64], 4)
		refs := make([]mmodel, 4)
		for i := range refs {
			refs[i] = mmodel{vals: map[int64]int64{}}
		}
		steps := 3000
		if testing.Short() {
			steps = 600
		}
		for step := range steps {
			i := rng.IntN(len(pool))
			k := rng.Int64N(keys)
			switch op := rng.IntN(10); {
			case op < 5:
				v := rng.Int64()
				pool[i].M_put(nil, k, v)
				refs[i].put(k, v)
			case op < 6 && rng.IntN(2) == 0:
				fail := rng.IntN(4) == 0
				r := Map_update(nil, &pool[i], k, 5, func(_ *Task, v *int64) Result[int64, string] {
					*v += 3
					if fail {
						return Err[int64]("no")
					}
					return Ok[int64, string](*v)
				})
				if r.Ok == fail {
					t.Fatalf("seed %d step %d: update result %v", seed, step, r)
				}
				if !fail {
					old, ok := refs[i].vals[k]
					if !ok {
						old = 5
					}
					refs[i].put(k, old+3)
				}
			case op < 7:
				got := pool[i].M_remove(nil, k)
				want, ok := refs[i].vals[k]
				if got.Some != ok || got.V != want {
					t.Fatalf("seed %d step %d: remove(%d) = %v, want %d %v", seed, step, k, got, want, ok)
				}
				refs[i].remove(k)
			case op < 8:
				j := rng.IntN(len(pool))
				pool[j] = pool[i]
				refs[j] = refs[i].clone()
			case op < 9:
				var u Map[int64, int64]
				pool[i] = u.With(k, 1)
				refs[i] = mmodel{vals: map[int64]int64{}}
				refs[i].put(k, 1)
			default:
				// A put that a copy makes and then drops: the original
				// must not see it.
				c := pool[i]
				c.M_put(nil, k, -1)
			}
			if step%7 == 0 || step == steps-1 {
				for j := range pool {
					checkModel(t, "after step", pool[j], refs[j], keys)
				}
			}
		}
	}
}

// TestMapHeavyRewrite drives one key range through many overwrites (log
// compaction) and long fork chains, checking a snapshot taken at each
// depth.
func TestMapHeavyRewrite(t *testing.T) {
	var m Map[int64, int64]
	ref := mmodel{vals: map[int64]int64{}}
	var snaps []Map[int64, int64]
	var refs []mmodel
	for round := range int64(30) {
		snap := m
		snaps = append(snaps, snap)
		refs = append(refs, ref.clone())
		// Write through the snapshot first half the time, so m becomes
		// stale and forks.
		if round%2 == 0 {
			snap.M_put(nil, 1000+round, round)
		}
		for i := range int64(500) {
			k := (i * 7) % 97
			m.M_put(nil, k, round*1000+i)
			ref.put(k, round*1000+i)
			if i%11 == 0 {
				m.M_remove(nil, (k+3)%97)
				ref.remove((k + 3) % 97)
			}
		}
	}
	checkModel(t, "final", m, ref, 1100)
	for i := range snaps {
		checkModel(t, "snapshot", snaps[i], refs[i], 1100)
	}
}

// TestMapLargeChurnedLog iterates root table views whose logs hold
// overwrites and tombstones past the stack buffers of eachRoot (512
// entries), and a stale snapshot of the same table.
func TestMapLargeChurnedLog(t *testing.T) {
	var m Map[int64, int64]
	ref := mmodel{vals: map[int64]int64{}}
	for k := range int64(400) {
		m.M_put(nil, k, k)
		ref.put(k, k)
	}
	snap, snapRef := m, ref.clone()
	for k := int64(0); k < 300; k++ {
		m.M_put(nil, (k*7)%400, -k)
		ref.put((k*7)%400, -k)
	}
	for k := int64(0); k < 20; k++ {
		m.M_remove(nil, k*13)
		ref.remove(k * 13)
	}
	m.M_put(nil, 13, 99) // back after its removal: last in order
	ref.put(13, 99)
	if m.t.parent != nil || len(m.es) <= 512 || len(m.es) == m.n {
		t.Fatalf("setup: parent %v log %d live %d, want a root log over 512 with garbage", m.t.parent != nil, len(m.es), m.n)
	}
	checkModel(t, "churned", m, ref, 400)
	snap.M_put(nil, 1, 5) // the snapshot is stale now: it forks
	snapRef.put(1, 5)
	checkModel(t, "snapshot", snap, snapRef, 400)
	checkModel(t, "churned after fork", m, ref, 400)
}

// TestMapConcurrentReaders reads snapshots on other goroutines while the
// owner keeps appending (run under -race by the CI race test).
func TestMapConcurrentReaders(t *testing.T) {
	var m Map[int64, int64]
	for i := range int64(1000) {
		m.M_put(nil, i, i)
	}
	var wg sync.WaitGroup
	for r := range 4 {
		snap := m
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				var sum int64
				for k, v := range snap.All() {
					if k != v {
						t.Errorf("reader %d: %d=%d", r, k, v)
						return
					}
					sum += v
				}
				if sum != 999*1000/2 || snap.M_len(nil) != 1000 {
					t.Errorf("reader %d: sum %d len %d", r, sum, snap.M_len(nil))
					return
				}
				for k := range int64(1000) {
					if snap.M_get(nil, k).V != k {
						t.Errorf("reader %d: get %d", r, k)
						return
					}
				}
			}
		}()
	}
	// Writers: the owner, and a second copy that loses the claim and
	// forks, both racing with the readers.
	other := m
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range int64(5000) {
			other.M_put(nil, i%1500, -i)
		}
	}()
	for i := range int64(20000) {
		m.M_put(nil, i%1200, i+1)
		if i%5 == 0 {
			m.M_remove(nil, (i+1)%1200)
		}
	}
	wg.Wait()
}

func TestSetValueSemantics(t *testing.T) {
	var s Set[string]
	s.M_add(nil, "a")
	s.M_add(nil, "b")
	c := s
	c.M_add(nil, "c")
	c.M_remove(nil, "a")
	s.M_add(nil, "d")
	if got := Debug(s); got != `#{"a", "b", "d"}` {
		t.Fatalf("s = %s", got)
	}
	if got := Debug(c); got != `#{"b", "c"}` {
		t.Fatalf("c = %s", got)
	}
}

// collide hashes every value alike, so every key shares one probe chain.
type collide struct{ n int64 }

func (c collide) XoHash() uint64  { return 42 }
func (c collide) XoEq(o any) bool { p, ok := o.(collide); return ok && p.n == c.n }

func TestMapHashCollisions(t *testing.T) {
	var m Map[collide, int64]
	for i := range int64(200) {
		m.M_put(nil, collide{i}, i)
	}
	snap := m
	for i := range int64(200) {
		if i%3 == 0 {
			m.M_remove(nil, collide{i})
		} else {
			m.M_put(nil, collide{i}, -i)
		}
	}
	for i := range int64(200) {
		if got := snap.M_get(nil, collide{i}); !got.Some || got.V != i {
			t.Fatalf("snapshot get %d = %v", i, got)
		}
		got := m.M_get(nil, collide{i})
		if i%3 == 0 && got.Some || i%3 != 0 && got.V != -i {
			t.Fatalf("get %d = %v", i, got)
		}
	}
}
