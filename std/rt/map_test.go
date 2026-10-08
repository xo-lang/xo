package rt

import "testing"

func BenchmarkMapPutInt(b *testing.B) {
	var m Map[int64, int64]
	for i := 0; i < b.N; i++ {
		m.M_put(nil, int64(i%50000), int64(i))
	}
}

func BenchmarkMapGetInt(b *testing.B) {
	var m Map[int64, int64]
	for i := 0; i < 50000; i++ {
		m.M_put(nil, int64(i), int64(i))
	}
	b.ResetTimer()
	var s int64
	for i := 0; i < b.N; i++ {
		s += m.M_get(nil, int64(i%50000)).V
	}
	_ = s
}

func BenchmarkMapPutStr(b *testing.B) {
	keys := make([]string, 1000)
	for i := range keys {
		keys[i] = "key" + IntStr(int64(i))
	}
	var m Map[string, int64]
	for i := 0; i < b.N; i++ {
		m.M_put(nil, keys[i%1000], int64(i))
	}
}

func BenchmarkEqInt(b *testing.B) {
	n := 0
	for i := 0; i < b.N; i++ {
		if Eq(int64(i), int64(i+1000)) {
			n++
		}
	}
	_ = n
}

func BenchmarkHashInt(b *testing.B) {
	var h uint64
	for i := 0; i < b.N; i++ {
		h += Hash(int64(i + 1000))
	}
	_ = h
}

func BenchmarkMapUpdate(b *testing.B) {
	var m Map[int64, int64]
	f := func(_ *Task, v *int64) Result[Unit, Never] { *v++; return Ok[Unit, Never](Unit{}) }
	for i := 0; i < b.N; i++ {
		Map_update(nil, &m, int64(i%50000), 0, f)
	}
}

func BenchmarkMapIter(b *testing.B) {
	var m Map[int64, int64]
	for i := 0; i < 50000; i++ {
		m.M_put(nil, int64(i), int64(i))
	}
	b.ResetTimer()
	var s int64
	for i := 0; i < b.N; i++ {
		for k, v := range m.All() {
			s += k + v
		}
	}
	_ = s
}

func BenchmarkMapCopyMutate(b *testing.B) {
	var m Map[int64, int64]
	for i := 0; i < 50000; i++ {
		m.M_put(nil, int64(i), int64(i))
	}
	b.ResetTimer()
	var s int64
	for i := 0; i < b.N; i++ {
		c := m
		for k := int64(0); k < 1000; k++ {
			c.M_put(nil, k, -k)
		}
		s += m.M_get(nil, 7).V + c.M_get(nil, 7).V
	}
	_ = s
}

// BenchmarkMapGetStale reads a map whose copy has since overwritten every
// key (each lookup follows one prev link past the copy's entries).
func BenchmarkMapGetStale(b *testing.B) {
	var m Map[int64, int64]
	for i := 0; i < 50000; i++ {
		m.M_put(nil, int64(i), int64(i))
	}
	c := m
	for i := 0; i < 50000; i++ {
		c.M_put(nil, int64(i), -1)
	}
	b.ResetTimer()
	var s int64
	for i := 0; i < b.N; i++ {
		s += m.M_get(nil, int64(i%50000)).V
	}
	_ = s
}

// BenchmarkMapGetFork reads a stale copy that forked: half of the keys
// are in its own table, the rest in the parent view.
func BenchmarkMapGetFork(b *testing.B) {
	var m Map[int64, int64]
	for i := 0; i < 50000; i++ {
		m.M_put(nil, int64(i), int64(i))
	}
	c := m
	c.M_put(nil, 0, 0)
	for i := 0; i < 50000; i += 2 {
		m.M_put(nil, int64(i), -1)
	}
	b.ResetTimer()
	var s int64
	for i := 0; i < b.N; i++ {
		s += m.M_get(nil, int64(i%50000)).V
	}
	_ = s
}

// BenchmarkMapCopy copies a map value into a slice of values (header copy).
func BenchmarkMapCopy(b *testing.B) {
	var m Map[int64, int64]
	for i := 0; i < 50000; i++ {
		m.M_put(nil, int64(i), int64(i))
	}
	out := make([]Map[int64, int64], 1024)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out[i&1023] = m
	}
}

// BenchmarkMapBranch makes one-key edits of the same 50k map.
func BenchmarkMapBranch(b *testing.B) {
	var m Map[int64, int64]
	for i := 0; i < 50000; i++ {
		m.M_put(nil, int64(i), int64(i))
	}
	b.ResetTimer()
	var s int64
	for i := 0; i < b.N; i++ {
		c := m
		c.M_put(nil, int64(50000+i), 1)
		s += c.M_len(nil)
	}
	_ = s
}

func BenchmarkSetAdd(b *testing.B) {
	var s Set[int64]
	for i := 0; i < b.N; i++ {
		s.M_add(nil, int64(i%50000))
	}
}
