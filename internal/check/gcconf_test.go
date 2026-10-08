package check

import (
	"strings"
	"testing"
)

// The `gc` lines of xo.mod (decision 0065).
func TestGCConfigRecorded(t *testing.T) {
	cases := []struct {
		mod  string
		want GCConfig
	}{
		{"module example.com/a\n", NoGCConfig},
		{"module example.com/a\ngc min_heap 64MiB\n", GCConfig{Percent: -1, MinHeap: 64 << 20, MemoryLimit: -1}},
		{"module example.com/a\ngc percent 200\ngc memory_limit 1GiB\n", GCConfig{Percent: 200, MinHeap: -1, MemoryLimit: 1 << 30}},
		{"module example.com/a\ngc percent off\ngc memory_limit 500MB\ngc min_heap 0\n", GCConfig{Percent: GCOff, MinHeap: 0, MemoryLimit: 500e6}},
		{"module example.com/a\ngc min_heap 4096\n", GCConfig{Percent: -1, MinHeap: 4096, MemoryLimit: -1}},
	}
	for _, tc := range cases {
		dir := writeTree(t, map[string]string{"xo.mod": tc.mod, "main.xo": editionMain})
		res, err := Check([]string{dir})
		if err != nil {
			t.Fatalf("%q: %v", tc.mod, err)
		}
		if got := res.Modules[0].GC; got != tc.want {
			t.Errorf("%q: gc %+v, want %+v", tc.mod, got, tc.want)
		}
	}
}

func TestGCConfigInvalid(t *testing.T) {
	for _, line := range []string{"gc percent 0", "gc percent lots", "gc min_heap 12 MiB", "gc min_heap -1", "gc min_heap 3XB", "gc arenas on", "gc percent"} {
		dir := writeTree(t, map[string]string{"xo.mod": "module x.com/a\n" + line + "\n", "main.xo": editionMain})
		_, err := Check([]string{dir})
		if err == nil || !strings.Contains(err.Error(), "gc") {
			t.Errorf("%q: err = %v, want a gc error", line, err)
		}
	}
}
