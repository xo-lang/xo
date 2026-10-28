package arm64

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestStdFactsVerify: a cold build's std facts (XO_STD_FACTS_DUMP)
// verify against a warm build's (XO_STD_FACTS_VERIFY), and a facts file with one
// borrowed parameter made owned fails the build naming it. A std
// function's borrowed parameters are its calling convention: decision
// 0388's stage 3 oracle made the runtime's owned and the HTTP server
// faulted on its first request (2026-10-27), which nothing but this
// comparison sees before the program runs.
func TestStdFactsVerify(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-stdfacts-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	prog := filepath.Join(work, "prog")
	if err := os.MkdirAll(prog, 0o755); err != nil {
		t.Fatal(err)
	}
	src := "fn main(os: Os) {\n  var xs: List[Str] = []\n  for i in 0..10 {\n    xs.push(\"${i}\")\n  }\n  os.stdio.println(xs.join(\",\"))\n}\n"
	if err := os.WriteFile(filepath.Join(prog, "main.xo"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	build := func(env ...string) (string, error) {
		cmd := testtmp.Guard(exec.Command(xo, "build", "--backend=arm64", "-o", filepath.Join(work, "bin"), "."))
		cmd.Dir = prog
		cmd.Env = append(os.Environ(), append([]string{"XO_BACKEND_CACHE_DIR=" + filepath.Join(work, "cache")}, env...)...)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	// the dump on a cold build, the verify on a warm one: a warm build's
	// std functions are undecoded shells, and their facts must be a cold
	// build's (fold_known_calls's constant results of the target
	// functions were missing from them, 2026-10-28)
	facts := filepath.Join(work, "facts.stdf")
	if out, err := build("XO_STD_FACTS_DUMP=" + facts); err != nil {
		t.Fatalf("dump: %v\n%s", err, out)
	}
	if out, err := build("XO_STD_FACTS_VERIFY=" + facts); err != nil {
		t.Fatalf("the build's own facts do not verify: %v\n%s", err, out)
	}
	d, err := os.ReadFile(facts)
	if err != nil {
		t.Fatal(err)
	}
	name, bad, err := ownOneBorrowed(d)
	if err != nil {
		t.Fatal(err)
	}
	tampered := filepath.Join(work, "owned.stdf")
	if err := os.WriteFile(tampered, bad, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := build("XO_STD_FACTS_VERIFY=" + tampered)
	if err == nil {
		t.Fatalf("facts with %s's borrowed parameter made owned verified\n%s", name, out)
	}
	if !strings.Contains(out, name+": borrowed") {
		t.Fatalf("the failure does not name %s's borrowed parameters:\n%s", name, out)
	}
}

// ownOneBorrowed returns the first function of a std facts file
// (compiler/ssa stdfacts.xo's codec) with a borrowed parameter, and the
// file with that parameter made owned.
func ownOneBorrowed(d []byte) (string, []byte, error) {
	at := 0
	uv := func() (int, error) {
		v, sh := 0, 0
		for {
			if at >= len(d) || sh > 63 {
				return 0, errors.New("std facts: truncated varint")
			}
			c := d[at]
			at++
			v |= int(c&127) << sh
			if c < 128 {
				return v, nil
			}
			sh += 7
		}
	}
	bytesAt := func() (int, int, error) {
		n, err := uv()
		if err != nil || at+n > len(d) {
			return 0, 0, errors.New("std facts: truncated bytes")
		}
		s := at
		at += n
		return s, n, nil
	}
	skipBools := func() ([]int, error) {
		n, err := uv()
		if err != nil {
			return nil, err
		}
		var pos []int
		for i := 0; i < n; i++ {
			pos = append(pos, at)
			if _, err := uv(); err != nil {
				return nil, err
			}
		}
		return pos, nil
	}
	if _, _, err := bytesAt(); err != nil { // the version
		return "", nil, err
	}
	n, err := uv()
	if err != nil {
		return "", nil, err
	}
	for i := 0; i < n; i++ {
		s, l, err := bytesAt()
		if err != nil {
			return "", nil, err
		}
		name := string(d[s : s+l])
		if _, _, err := bytesAt(); err != nil { // body facts
			return "", nil, err
		}
		np, err := uv()
		if err != nil {
			return "", nil, err
		}
		for j := 0; j < np; j++ {
			if _, err := uv(); err != nil {
				return "", nil, err
			}
		}
		borrowed, err := skipBools()
		if err != nil {
			return "", nil, err
		}
		for _, p := range borrowed {
			if d[p] == 1 {
				out := bytes.Clone(d)
				out[p] = 0
				return name, out, nil
			}
		}
		if _, err := skipBools(); err != nil { // unique
			return "", nil, err
		}
		if _, err := skipBools(); err != nil { // catches, faults, cancels
			return "", nil, err
		}
		tag, err := uv()
		if err != nil {
			return "", nil, err
		}
		switch tag {
		case 1, 2:
			_, err = uv()
		case 3, 5:
			_, _, err = bytesAt()
		case 4:
			if _, err = uv(); err == nil {
				_, err = uv()
			}
		}
		if err != nil {
			return "", nil, err
		}
		if _, _, err := bytesAt(); err != nil { // inline body
			return "", nil, err
		}
	}
	return "", nil, errors.New("std facts: no function with a borrowed parameter")
}
