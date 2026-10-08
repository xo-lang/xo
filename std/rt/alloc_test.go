package rt

import "testing"

// Allocation budgets of the per request paths of an HTTP server
// (bench/perf/http/allocs.sh measures the whole request). Each test pins
// the count so a change that adds an allocation to a hot path fails here.

var sinkStrMap Map[string, string]

// A small map is its table and its log array: the first index and the
// first array header live in the table.
func TestAllocsSmallMap(t *testing.T) {
	n := testing.AllocsPerRun(100, func() {
		var m Map[string, string]
		m.M_put(nil, "Content-Type", "application/json")
		m.M_put(nil, "Accept", "*/*")
		m.M_put(nil, "User-Agent", "x")
		if m.M_get(nil, "Accept").V != "*/*" {
			t.Fatal("lookup")
		}
		sinkStrMap = m
	})
	if n > 2 {
		t.Errorf("small Map[Str, Str]: %v allocations, want at most 2", n)
	}
}
