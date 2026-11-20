package main

import (
	"reflect"
	"testing"
)

func TestParseAB(t *testing.T) {
	out := "building...\nab bench/perf/json/enc go 12.0 a 13.2 b 12.6 ra 1.100 rb 1.050 rounds 30\nbench/perf/x/y: time: B's Xo program differs\nab bad go x a 1 b 1 ra 1 rb 1 rounds 3\n"
	rows, bad := parseAB(out)
	want := []twinRow{{name: "bench/perf/json/enc", goMs: 12, aMs: 13.2, bMs: 12.6, ra: 1.1, rb: 1.05, rounds: 30}}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("rows = %+v, want %+v", rows, want)
	}
	if len(bad) != 2 {
		t.Errorf("bad = %q, want the time failure and the unparsable line", bad)
	}
}

func TestAsmCounts(t *testing.T) {
	asm := "\t.section __TEXT\n// func main\n\t.p2align 2\n_xo_f_main:\n\tstp x29, x30, [sp, #-16]!\n\tbl _f\n\tret\n// func strconv.itoa\n\tmov x0, x1\n\tret\n// func Point.len\n\tret\n"
	got := asmCounts(asm)
	want := map[string]int{"main": 3, "strconv.itoa": 2, "Point.len": 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("asmCounts = %v, want %v", got, want)
	}
	if o := own(got, map[string]bool{"Point": true}); !reflect.DeepEqual(o, map[string]int{"main": 3, "Point.len": 1}) {
		t.Errorf("own = %v", o)
	}
}

func TestParsePhases(t *testing.T) {
	got := parsePhases("phase check 120 ms heap_mb=10 peak_mb=20\nphase codegen 300.5 ms heap_mb=1 peak_mb=2\nother\nphase check 5 ms\n")
	want := map[string]float64{"check": 125, "codegen": 300.5}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parsePhases = %v, want %v", got, want)
	}
}

func TestGeomean(t *testing.T) {
	if g := geomean([]float64{2, 0.5}); g < 0.9999 || g > 1.0001 {
		t.Errorf("geomean 2 0.5 = %v", g)
	}
}
