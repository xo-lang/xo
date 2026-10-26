package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDocPreludeMethods covers `xo doc Map` (2026-10-21, reported by Xo
// Lang 3): a prelude type lists its methods (Map.get, Map.put ...), as a
// user type does, and `xo check --dir=DIR` checks DIR as the directory
// argument does.
func TestDocPreludeMethods(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module example.com/dp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte("fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)
	var out, errb bytes.Buffer
	if code := runXo(t, []string{"doc", "--dir=.", "--json", "Map"}, &out, &errb); code != 0 {
		t.Fatalf("xo doc Map: exit %d\n%s%s", code, out.String(), errb.String())
	}
	for _, want := range []string{`"name":"Map.get"`, `"name":"Map.contains"`} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("xo doc Map lacks %s:\n%.600s", want, out.String())
		}
	}
	out.Reset()
	errb.Reset()
	if code := runXo(t, []string{"check", "--dir=.", "--json"}, &out, &errb); code != 0 || !strings.Contains(out.String(), `"modules"`) {
		t.Errorf("xo check --dir=.: exit %d\n%s%s", code, out.String(), errb.String())
	}
	out.Reset()
	errb.Reset()
	if code := runXo(t, []string{"check"}, &out, &errb); code != 2 || !strings.Contains(errb.String(), "no directories given") {
		t.Errorf("xo check with nothing: exit %d\n%s", code, errb.String())
	}
}

// TestDocRefusesWideDir: a --dir with thousands of .xo files below it
// (a scratch or home directory) is refused with a message instead of
// loading every tree under it (2026-10-21: `xo doc --dir=/private/tmp Map`
// reached 220 GB).
func TestDocRefusesWideDir(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 6; i++ {
		sub := filepath.Join(dir, "d"+string(rune('a'+i)))
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		for j := 0; j < 1001; j++ {
			name := filepath.Join(sub, "f"+strings.Repeat("x", j%7)+string(rune('a'+j%26))+itoaT(j)+".xo")
			if err := os.WriteFile(name, nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	chdir(t, dir)
	for _, cmd := range []string{"doc", "def", "refs"} {
		var out, errb bytes.Buffer
		code := runXo(t, []string{cmd, "--dir=.", "Map"}, &out, &errb)
		if code == 0 || !strings.Contains(out.String()+errb.String(), "give a module or repository directory") {
			t.Errorf("xo %s on a wide directory: exit %d\n%.300s%.300s", cmd, code, out.String(), errb.String())
		}
	}
}

func itoaT(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
