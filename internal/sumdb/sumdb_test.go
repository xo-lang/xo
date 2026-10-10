package sumdb

import (
	"crypto/rand"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

func leavesN(n int) []Hash {
	var out []Hash
	for i := 0; i < n; i++ {
		out = append(out, RecordHash([]byte(fmt.Sprintf("record %d\n", i))))
	}
	return out
}

// TestProofs checks every inclusion and consistency proof of trees up to
// 40 records, and that a changed leaf, root, or proof fails.
func TestProofs(t *testing.T) {
	all := leavesN(40)
	for n := 1; n <= len(all); n++ {
		leaves := all[:n]
		root := TreeHash(leaves)
		for i := 0; i < n; i++ {
			p := InclusionProof(leaves, int64(i))
			if err := VerifyInclusion(p, int64(n), int64(i), leaves[i], root); err != nil {
				t.Fatalf("inclusion %d of %d: %v", i, n, err)
			}
			if VerifyInclusion(p, int64(n), int64(i), RecordHash([]byte("other")), root) == nil {
				t.Fatalf("inclusion %d of %d: a changed leaf verifies", i, n)
			}
			if len(p) > 0 {
				bad := append([]Hash(nil), p...)
				bad[0][0] ^= 1
				if VerifyInclusion(bad, int64(n), int64(i), leaves[i], root) == nil {
					t.Fatalf("inclusion %d of %d: a changed proof verifies", i, n)
				}
			}
		}
		for m := 1; m <= n; m++ {
			p := ConsistencyProof(leaves, int64(m))
			old := TreeHash(leaves[:m])
			if err := VerifyConsistency(p, int64(m), int64(n), old, root); err != nil {
				t.Fatalf("consistency %d of %d: %v", m, n, err)
			}
			if m < n {
				forked := append([]Hash(nil), leaves[:m]...)
				forked[m-1] = RecordHash([]byte("forked"))
				if VerifyConsistency(p, int64(m), int64(n), TreeHash(forked), root) == nil {
					t.Fatalf("consistency %d of %d: a forked old tree verifies", m, n)
				}
			}
		}
	}
}

func TestKeys(t *testing.T) {
	signer, verifier, err := GenerateKey("sum.example.com", rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	s, err := ParseSigner(signer)
	if err != nil {
		t.Fatal(err)
	}
	v, err := ParseVerifier(verifier)
	if err != nil {
		t.Fatal(err)
	}
	note := s.SignTree(Tree{N: 3, Hash: TreeHash(leavesN(3))})
	if tr, err := v.VerifyTree(note); err != nil || tr.N != 3 {
		t.Fatalf("VerifyTree = %v, %v", tr, err)
	}
	tampered := strings.Replace(string(note), "\n3\n", "\n4\n", 1)
	if _, err := v.VerifyTree([]byte(tampered)); err == nil {
		t.Fatal("a tampered tree head verifies")
	}
	_, other, _ := GenerateKey("sum.example.com", rand.Reader)
	ov, _ := ParseVerifier(other)
	if _, err := ov.VerifyTree(note); err == nil {
		t.Fatal("a tree head verifies with another key")
	}
	// a key id that does not match is rejected
	parts := strings.SplitN(verifier, "+", 3)
	if _, err := ParseVerifier(parts[0] + "+00000000+" + parts[2]); err == nil {
		t.Fatal("a wrong key id parses")
	}
	if _, _, err := ParseConfig("off"); err == nil {
		t.Fatal("ParseConfig accepts a bare word")
	}
	if _, u, err := ParseConfig(verifier); err != nil || u != "https://sum.example.com" {
		t.Fatalf("ParseConfig default URL = %q, %v", u, err)
	}
}

func record(path, version, tree string) []byte {
	return []byte(fmt.Sprintf("%s %s h1:%s\n%s %s/xo.mod h1:mod%s\n", path, version, tree, path, version, tree))
}

func want(path, version, tree string) []string {
	return strings.Split(strings.TrimSuffix(string(record(path, version, tree)), "\n"), "\n")
}

func newLog(t *testing.T, lookup func(p, v string) ([]byte, error)) (*Server, *httptest.Server, string) {
	t.Helper()
	signer, verifier, err := GenerateKey("sum.test", rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := ParseSigner(signer)
	srv := NewServer(s, lookup)
	hs := httptest.NewServer(srv)
	t.Cleanup(hs.Close)
	return srv, hs, verifier
}

func client(t *testing.T, verifier, url, state string) *Client {
	t.Helper()
	v, _, err := ParseConfig(verifier + " " + url)
	if err != nil {
		t.Fatal(err)
	}
	return &Client{URL: url, Verifier: v, StateDir: state}
}

func TestClientServer(t *testing.T) {
	srv, hs, verifier := newLog(t, func(p, v string) ([]byte, error) {
		if p == "example.com/missing" {
			return nil, fmt.Errorf("no such module")
		}
		return record(p, v, "good"+v), nil
	})
	state := t.TempDir()
	c := client(t, verifier, hs.URL, state)
	for _, v := range []string{"v1.0.0", "v1.1.0", "v1.2.0"} {
		if err := c.Check("example.com/Acme", v, want("example.com/Acme", v, "good"+v)); err != nil {
			t.Fatalf("%s: %v", v, err)
		}
	}
	// a record the fetched tree does not match
	err := c.Check("example.com/Acme", "v1.0.0", want("example.com/Acme", "v1.0.0", "evil"))
	if e, ok := err.(*Error); !ok || !e.Security || !strings.Contains(e.Msg, "records") {
		t.Fatalf("mismatch: %v", err)
	}
	// a version the log cannot find is not a security failure
	err = c.Check("example.com/missing", "v1.0.0", want("example.com/missing", "v1.0.0", "x"))
	if e, ok := err.(*Error); !ok || e.Security {
		t.Fatalf("missing: %v", err)
	}
	// another key
	_, other, _ := GenerateKey("sum.test", rand.Reader)
	err = client(t, other, hs.URL, t.TempDir()).Check("example.com/Acme", "v1.0.0", want("example.com/Acme", "v1.0.0", "goodv1.0.0"))
	if e, ok := err.(*Error); !ok || !e.Security {
		t.Fatalf("other key: %v", err)
	}
	// a log that drops records (rolled back) after the client saw 3
	if srv.Size() != 3 {
		t.Fatalf("size %d", srv.Size())
	}
}

// TestFork serves two logs under one key: the second rewrote the first
// record. A client that saw the first tree rejects the second.
func TestFork(t *testing.T) {
	signer, verifier, _ := GenerateKey("sum.test", rand.Reader)
	s, _ := ParseSigner(signer)
	honest := NewServer(s, nil)
	forked := NewServer(s, nil)
	honest.Add("example.com/a", "v1.0.0", record("example.com/a", "v1.0.0", "a"))
	honest.Add("example.com/b", "v1.0.0", record("example.com/b", "v1.0.0", "b"))
	forked.Add("example.com/a", "v1.0.0", record("example.com/a", "v1.0.0", "EVIL"))
	forked.Add("example.com/b", "v1.0.0", record("example.com/b", "v1.0.0", "b"))
	forked.Add("example.com/c", "v1.0.0", record("example.com/c", "v1.0.0", "c"))
	h1 := httptest.NewServer(honest)
	defer h1.Close()
	h2 := httptest.NewServer(forked)
	defer h2.Close()
	state := t.TempDir()
	if err := client(t, verifier, h1.URL, state).Check("example.com/b", "v1.0.0", want("example.com/b", "v1.0.0", "b")); err != nil {
		t.Fatal(err)
	}
	err := client(t, verifier, h2.URL, state).Check("example.com/c", "v1.0.0", want("example.com/c", "v1.0.0", "c"))
	if e, ok := err.(*Error); !ok || !e.Security || !strings.Contains(e.Msg, "does not extend") {
		t.Fatalf("fork: %v", err)
	}
	// a smaller tree than the one seen must be a prefix of it
	small := NewServer(s, nil)
	small.Add("example.com/b", "v1.0.0", record("example.com/b", "v1.0.0", "b"))
	h3 := httptest.NewServer(small)
	defer h3.Close()
	err = client(t, verifier, h3.URL, state).Check("example.com/b", "v1.0.0", want("example.com/b", "v1.0.0", "b"))
	if e, ok := err.(*Error); !ok || !e.Security {
		t.Fatalf("rollback: %v", err)
	}
}

func TestEscape(t *testing.T) {
	for _, p := range []string{"github.com/Acme/Billing", "example.com/x", "v1.0.0-RC1"} {
		e := EscapePath(p)
		if strings.ToLower(e) != e {
			t.Errorf("EscapePath(%q) = %q has upper case", p, e)
		}
		back, err := UnescapePath(e)
		if err != nil || back != p {
			t.Errorf("UnescapePath(%q) = %q, %v", e, back, err)
		}
	}
	for _, bad := range []string{"A", "!", "!1"} {
		if _, err := UnescapePath(bad); err == nil {
			t.Errorf("UnescapePath(%q) succeeds", bad)
		}
	}
}
