package rt

import (
	"math"
	"strings"
	"sync"
	"testing"
)

// Atomic[Int] and Atomic[Bool] over one atomic.Int64: every operation,
// add's overflow fault, and no lost update between goroutines (-race).

func TestAtomicInt(t *testing.T) {
	a := Atomic_new[int64](nil, -7)
	if v := a.M_get(nil); v != -7 {
		t.Fatalf("get %d", v)
	}
	a.M_set(nil, math.MinInt64)
	if v := a.M_swap(nil, 5); v != math.MinInt64 {
		t.Fatalf("swap returned %d", v)
	}
	if a.M_compare_and_swap(nil, 4, 9) || !a.M_compare_and_swap(nil, 5, 9) {
		t.Fatal("compare_and_swap")
	}
	if v := Atomic_add(nil, a, -3); v != 6 {
		t.Fatalf("add %d", v)
	}
	if v := Atomic_max(nil, a, 2); v != 6 {
		t.Fatalf("max below %d", v)
	}
	if v := Atomic_max(nil, a, 11); v != 11 || a.M_get(nil) != 11 {
		t.Fatalf("max above %d", v)
	}
	var b strings.Builder
	a.XoDebug(&b)
	if b.String() != "Atomic(11)" {
		t.Fatalf("debug %q", b.String())
	}
}

func TestAtomicBool(t *testing.T) {
	a := Atomic_new(nil, true)
	if !a.M_get(nil) {
		t.Fatal("get")
	}
	if !a.M_swap(nil, false) || a.M_get(nil) {
		t.Fatal("swap")
	}
	if a.M_compare_and_swap(nil, true, true) || !a.M_compare_and_swap(nil, false, true) || !a.M_get(nil) {
		t.Fatal("compare_and_swap")
	}
	var b strings.Builder
	a.XoDebug(&b)
	if b.String() != "Atomic(true)" {
		t.Fatalf("debug %q", b.String())
	}
}

func TestAtomicAddOverflowFaults(t *testing.T) {
	a := Atomic_new[int64](nil, math.MaxInt64)
	defer func() {
		if recover() == nil {
			t.Fatal("no fault on overflow")
		}
		if a.M_get(nil) != math.MaxInt64 {
			t.Fatalf("value changed to %d", a.M_get(nil))
		}
	}()
	Atomic_add(nil, a, 1)
}

func TestAtomicNoLostUpdate(t *testing.T) {
	a := Atomic_new[int64](nil, 0)
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 1000 {
				Atomic_add(nil, a, 1)
				Atomic_max(nil, a, int64(i))
			}
		}()
	}
	wg.Wait()
	if v := a.M_get(nil); v != 4000 {
		t.Fatalf("got %d, want 4000", v)
	}
}
