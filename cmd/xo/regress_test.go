package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestRegressionWritten checks that a shrunk contract fuzz failure becomes a
// regression test in the module (core 9.2), once, and that it fails.
func TestRegressionWritten(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds and runs a test binary")
	}
	dir := filepath.Join(t.TempDir(), "halver")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := "fn half(n: Int) -> Int\n  ensures result * 2 == n\n{\n  n / 2\n}\n\npub fn twice(n: Int) -> Int { half(n) * 4 }\n"
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	// contract fuzzing is the Go backend's (decision 0248: named, since
	// the default is native, where it is skipped)
	if code := runTest(t, []string{"--backend=go", dir}, &out, &errb); code == 0 {
		t.Fatalf("want the contract fuzz to fail\n%s%s", out.String(), errb.String())
	}
	got, err := os.ReadFile(filepath.Join(dir, regressFile))
	if err != nil {
		t.Fatalf("no regression file: %v\n%s%s", err, out.String(), errb.String())
	}
	if !strings.Contains(string(got), `test "regression: halver.half (seed `) || !strings.Contains(string(got), "half(arg_n)") {
		t.Fatalf("unexpected regression file:\n%s", got)
	}
	out.Reset()
	if code := runTest(t, []string{dir}, &out, &errb); code == 0 || !strings.Contains(out.String(), "--- FAIL: regression: halver.half") {
		t.Fatalf("want the regression test to fail\n%s", out.String())
	}
	again, _ := os.ReadFile(filepath.Join(dir, regressFile))
	if string(again) != string(got) {
		t.Fatalf("regression written twice:\n%s", again)
	}
}

// TestRegressionCrossBackend: contract fuzzing on the native backend
// (decision 0250) and on the Go backend write the same regression test
// for the same failure, and a regression recorded by one backend fails
// when the other replays it.
func TestRegressionCrossBackend(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds and runs test binaries on two backends")
	}
	native := "--backend=" + xotool.NativeBackend()
	src := "fn half(n: Int) -> Int\n  ensures result * 2 == n\n{\n  n / 2\n}\n\npub fn twice(n: Int) -> Int { half(n) * 4 }\n"
	files := map[string]string{}
	for _, c := range []struct{ rec, replay string }{{"--backend=go", native}, {native, "--backend=go"}} {
		dir := filepath.Join(t.TempDir(), "halver")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		var out, errb bytes.Buffer
		if code := runTest(t, []string{c.rec, dir}, &out, &errb); code == 0 || !strings.Contains(out.String(), "--- FAIL: contract fuzz: halver.half") {
			t.Fatalf("%s: want the contract fuzz to fail\n%s%s", c.rec, out.String(), errb.String())
		}
		got, err := os.ReadFile(filepath.Join(dir, regressFile))
		if err != nil {
			t.Fatalf("%s: no regression file: %v\n%s%s", c.rec, err, out.String(), errb.String())
		}
		files[c.rec] = string(got)
		out.Reset()
		if code := runTest(t, []string{c.replay, "--run=regression:", dir}, &out, &errb); code == 0 || !strings.Contains(out.String(), "--- FAIL: regression: halver.half") || !strings.Contains(out.String(), "ensures failed") {
			t.Fatalf("%s replaying %s's regression: want it to fail\n%s%s", c.replay, c.rec, out.String(), errb.String())
		}
	}
	if files["--backend=go"] != files[native] {
		t.Fatalf("the backends wrote different regression files:\n--- go\n%s--- %s\n%s", files["--backend=go"], native, files[native])
	}
}

// TestContractFuzzSmallInts: contract fuzzing draws small numbers on every
// backend (std/rt GenRand.Small, decision 0250), so a function that
// overflows only at the extremes, as a deposit adding to a balance,
// passes on both (it failed natively before the native generator had the
// small mode: the website tour's mutation example, 2026-10-17).
func TestContractFuzzSmallInts(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds and runs test binaries on two backends")
	}
	src := "struct Account {\n  owner: Str\n  balance: Int\n}\n\nfn (var a: Account) deposit(amount: Int)\n  requires amount > 0\n{\n  a.balance += amount\n}\n"
	for _, be := range []string{"--backend=go", "--backend=" + xotool.NativeBackend()} {
		dir := filepath.Join(t.TempDir(), "bank")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		var out, errb bytes.Buffer
		if code := runTest(t, []string{be, "-v", dir}, &out, &errb); code != 0 || !strings.Contains(out.String(), "--- PASS: contract fuzz: bank.Account.deposit") {
			t.Fatalf("%s: want the contract fuzz to pass\n%s%s", be, out.String(), errb.String())
		}
		if _, err := os.Stat(filepath.Join(dir, regressFile)); err == nil {
			t.Fatalf("%s: wrote a regression test", be)
		}
	}
}
