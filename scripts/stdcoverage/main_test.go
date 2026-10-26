package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWriteList checks the audit lists' format: one `module.symbol (kind)`
// an entry in the order given, and the `[impl]` mark where the run's
// interface marking says a method's name a covered interface method has
// (the mark the printed over list carries, kept so a sharded audit's lists
// can be merged without losing it).
func TestWriteList(t *testing.T) {
	tests := []struct {
		name  string
		keys  []string
		impls map[string]bool
		want  string
	}{
		{
			name: "empty",
			keys: nil,
			want: "",
		},
		{
			name:  "fn and one marked method",
			keys:  []string{"a.F (fn)", "b.G (method)"},
			impls: map[string]bool{"b.G (method)": true},
			want:  "a.F (fn)\nb.G (method)  [impl]\n",
		},
		{
			name:  "mark false",
			keys:  []string{"c.H (method)"},
			impls: map[string]bool{"c.H (method)": false},
			want:  "c.H (method)\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeList(dir, "over.txt", tt.keys, tt.impls)
			got, err := os.ReadFile(filepath.Join(dir, "over.txt"))
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("writeList = %q, want %q", string(got), tt.want)
			}
		})
	}
}
