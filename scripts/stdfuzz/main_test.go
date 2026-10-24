package main

import "testing"

// TestCompare: equal values agree, a different value diverges, and a
// backend with no line for the case diverges too.
func TestCompare(t *testing.T) {
	values := parseOut(map[string][]string{
		"arm64": {"a|1", "b|2"},
		"llvm":  {"a|1", "b|3"},
		"go":    {"a|1"},
	})
	backends := []string{"arm64", "llvm", "go"}
	if _, same := compare(backends, values, "a"); !same {
		t.Error("case a: equal values reported a divergence")
	}
	vals, same := compare(backends, values, "b")
	if same {
		t.Error("case b: 2 against 3 reported no divergence")
	}
	if vals["arm64"] != "2" || vals["llvm"] != "3" {
		t.Errorf("case b: values %v", vals)
	}
	if _, same := compare(backends, values, "c"); same {
		t.Error("case c: a missing line reported no divergence")
	}
}
