package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func module(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module example.com/play\nedition 2026\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.xo"), []byte(src), 0o644)
	return dir
}

// roundTrip runs a command and decodes its result as the browser does.
func roundTrip(t *testing.T, args ...string) map[string]any {
	t.Helper()
	out, err := run(args)
	if err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	data, _ := json.Marshal(out)
	var m map[string]any
	json.Unmarshal(data, &m)
	return m
}

func TestCheck(t *testing.T) {
	dir := module(t, "fn parse_port(s: Str) -> Int ! Error {\n  Int.parse(s).or_err(\"not a number\")?\n}\n\nfn main(os: Os) {\n  parse_port(\"8080\")\n}\n")
	m := roundTrip(t, "check", dir)
	ds := m["diagnostics"].([]any)
	if len(ds) != 1 || ds[0].(map[string]any)["code"] != "XO0201" {
		t.Fatalf("diagnostics = %v, want one XO0201", ds)
	}
}

func TestFormat(t *testing.T) {
	dir := module(t, "fn main(os: Os) {\nos.stdio.println(\"hi\")\n}\n")
	m := roundTrip(t, "fmt", filepath.Join(dir, "main.xo"))
	got, _ := os.ReadFile(filepath.Join(dir, "main.xo"))
	if m["changed"] != true || !strings.Contains(string(got), "\n  os.stdio.println") {
		t.Fatalf("changed = %v, source:\n%s", m["changed"], got)
	}
}

func TestGen(t *testing.T) {
	dir := module(t, "fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n")
	out := t.TempDir()
	m := roundTrip(t, "gen", dir, out)
	pkgs := m["packages"].([]any)
	if len(pkgs) != 2 {
		t.Fatalf("packages = %v, want the module and main", pkgs)
	}
	if p := pkgs[0].(map[string]any)["path"]; p != "xoprog/m/x_example_com_play" {
		t.Errorf("first package = %v, want the module before main", p)
	}
	if p := pkgs[1].(map[string]any)["path"]; p != "main" {
		t.Errorf("last package = %v, want main", p)
	}
	for _, f := range pkgs[1].(map[string]any)["files"].([]any) {
		if _, err := os.Stat(f.(string)); err != nil {
			t.Errorf("listed file not written: %v", err)
		}
	}
	if tags := m["tags"].([]any); len(tags) != 1 || tags[0] != "xo_nohttp" {
		t.Errorf("tags = %v, want [xo_nohttp]", tags)
	}
	if rt := m["runtime"].([]any); len(rt) != 0 {
		t.Errorf("runtime = %v, want none without use go", rt)
	}
}

func TestGenCheckFails(t *testing.T) {
	dir := module(t, "fn main(os: Os) {\n  let x = 1\n}\n")
	m := roundTrip(t, "gen", dir, t.TempDir())
	if _, ok := m["packages"]; ok {
		t.Fatalf("a program with errors produced packages: %v", m)
	}
	if len(m["diagnostics"].([]any)) == 0 {
		t.Fatal("no diagnostics")
	}
}

func TestGenTest(t *testing.T) {
	dir := module(t, "fn add(a: Int, b: Int) -> Int {\n  a + b\n}\n\ntest \"add\" {\n  expect(add(2, 2)) == 4\n}\n\nfn main(os: Os) {\n}\n")
	m := roundTrip(t, "gen", "--test", dir, t.TempDir())
	if _, ok := m["packages"]; !ok {
		t.Fatalf("no packages: %v", m)
	}
	if tags := m["tags"].([]any); len(tags) != 1 || tags[0] != "xo_nohttp" {
		t.Errorf("tags = %v, want [xo_nohttp] for tests that do not load std/http", tags)
	}
}
