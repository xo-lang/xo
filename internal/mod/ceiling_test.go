package mod

import (
	"strings"
	"testing"
)

func TestParseCeiling(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		ok       bool
	}{
		{"uses none", "uses none", true},
		{"uses net", "uses net", true},
		{"uses net, fs.read", "uses net, fs.read", true},
		{"uses (net, fs.read, net)", "uses net, fs.read", true},
		{"uses", "", false},
		{"uses ()", "", false},
		{"uses telepathy", "", false},
		{"with net", "", false},
	} {
		c, err := ParseCeiling(strings.Fields(tc.in))
		if (err == nil) != tc.ok {
			t.Errorf("%q: err %v", tc.in, err)
			continue
		}
		if tc.ok && c.String() != tc.want {
			t.Errorf("%q: %q, want %q", tc.in, c.String(), tc.want)
		}
	}
	c := &Ceiling{Effects: []string{"fs", "net.dial"}}
	for l, want := range map[string]bool{"fs": true, "fs.read": true, "net.dial": true, "net": false, "net.listen": false, "env": false} {
		if c.Admits(l) != want {
			t.Errorf("Admits(%q) = %v", l, !want)
		}
	}
	var none *Ceiling
	if !none.Admits("net") {
		t.Errorf("no ceiling admits everything")
	}
}

func TestRequireCeiling(t *testing.T) {
	src := "module example.com/app\n\nrequire github.com/acme/a v1.0.0 uses none // pure\nrequire github.com/acme/b v1.0.0 uses (net, fs.read)\nrequire github.com/acme/c v1.0.0\n"
	f, probs := Parse("xo.mod", []byte(src))
	if len(probs) > 0 {
		t.Fatal(probs[0])
	}
	got := []string{f.Requires[0].Ceiling.String(), f.Requires[1].Ceiling.String(), f.Requires[2].Ceiling.String()}
	if strings.Join(got, "|") != "uses none|uses net, fs.read|" {
		t.Errorf("ceilings: %q", got)
	}
	out := string(SetRequire([]byte(src), "github.com/acme/a", "v1.2.0"))
	if !strings.Contains(out, "require github.com/acme/a v1.2.0 uses none\n") {
		t.Errorf("SetRequire dropped the ceiling:\n%s", out)
	}
	_, probs = Parse("xo.mod", []byte("module example.com/app\nrequire github.com/acme/a v1.0.0 uses magic\n"))
	if len(probs) != 1 || probs[0].Module != "github.com/acme/a" {
		t.Errorf("malformed ceiling: %v", probs)
	}
}

func TestMajorSuffix(t *testing.T) {
	for _, tc := range []struct {
		path, base string
		n          int
	}{
		{"github.com/acme/billing", "github.com/acme/billing", 0},
		{"github.com/acme/billing/v2", "github.com/acme/billing", 2},
		{"github.com/acme/billing/v10", "github.com/acme/billing", 10},
		{"github.com/acme/billing/v1", "github.com/acme/billing/v1", 0},
		{"github.com/acme/billing/v02", "github.com/acme/billing/v02", 0},
		{"github.com/acme/billing/vx", "github.com/acme/billing/vx", 0},
	} {
		base, n := MajorSuffix(tc.path)
		if base != tc.base || n != tc.n {
			t.Errorf("MajorSuffix(%q) = %q, %d", tc.path, base, n)
		}
	}
	for _, tc := range []struct {
		path, v string
		ok      bool
	}{
		{"github.com/a/b", "v1.2.0", true},
		{"github.com/a/b", "v0.2.0", true},
		{"github.com/a/b", "v2.0.0", false},
		{"github.com/a/b/v2", "v2.1.0", true},
		{"github.com/a/b/v2", "v3.0.0", false},
		{"github.com/a/b/v2", "v1.0.0", false},
	} {
		if got := MajorMismatch(tc.path, tc.v) == ""; got != tc.ok {
			t.Errorf("MajorMismatch(%q, %q) ok = %v", tc.path, tc.v, got)
		}
	}
	t.Setenv("XO_GIT_REWRITE", "")
	t.Setenv("XO_GIT_BASE", "/repos")
	if got := RepoURL("github.com/a/b/v3"); got != "/repos/github.com/a/b" {
		t.Errorf("RepoURL of a /v3 path: %s", got)
	}
}
