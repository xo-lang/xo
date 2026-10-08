package build

import (
	"path/filepath"
	"testing"
)

func TestParseTargets(t *testing.T) {
	ts, err := ParseTargets("linux/amd64, wasi/wasm32,windows/arm64")
	if err != nil {
		t.Fatal(err)
	}
	want := []Target{{"linux", "amd64"}, {"wasip1", "wasm"}, {"windows", "arm64"}}
	if len(ts) != len(want) {
		t.Fatalf("got %v", ts)
	}
	for i := range want {
		if ts[i] != want[i] {
			t.Errorf("target %d: got %v, want %v", i, ts[i], want[i])
		}
	}
	all, err := ParseTargets("all")
	if err != nil || len(all) != 7 {
		t.Errorf("all: %v %v", all, err)
	}
	host, err := ParseTargets("")
	if err != nil || len(host) != 1 || host[0] != Host() {
		t.Errorf("default: %v %v", host, err)
	}
	for _, bad := range []string{"linux", "/amd64", "linux/"} {
		if _, err := ParseTargets(bad); err == nil {
			t.Errorf("%q: no error", bad)
		}
	}
}

func TestBinPath(t *testing.T) {
	cases := []struct {
		out  string
		t    Target
		many bool
		want string
	}{
		{"", Target{"linux", "amd64"}, false, filepath.Join("d", "bin", "app")},
		{"", Target{"windows", "amd64"}, false, filepath.Join("d", "bin", "app.exe")},
		{"o", Target{"darwin", "arm64"}, true, filepath.Join("o", "app-darwin-arm64")},
		{"o", Target{"wasip1", "wasm"}, true, filepath.Join("o", "app-wasip1-wasm.wasm")},
		{"x/app", Target{"linux", "amd64"}, false, "x/app"},
	}
	for _, c := range cases {
		if got := binPath("d", c.out, "app", c.t, c.many); got != c.want {
			t.Errorf("binPath(%q, %v, %v) = %q, want %q", c.out, c.t, c.many, got, c.want)
		}
	}
}
