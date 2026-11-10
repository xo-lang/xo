package main

import "testing"

// TestVetBufView covers XO0541 (gotchas/slice-view-of-reused-buffer.md):
// a slice of a raw.bytes_buf that a loop reads into again, kept past the
// iteration. The program names its own `raw.bytes_buf` (the check matches
// the call as written; std's netlink and resolver code are the real
// cases): the netlink shape (a struct of the slice pushed to an outer
// list), the resolver shape (a local bound to the slice, pushed), and
// the resolver's (a parser of a view of the buffer whose result is
// stored outside the loop), and three that must not report: a clone, a use inside the iteration, and a
// buffer the loop never reads into.
func TestVetBufView(t *testing.T) {
	src := `struct Raw {}

fn (r: Raw) bytes_buf(n: Int) -> Bytes { Bytes.zeroed(n) }

fn recv(b: Bytes) -> Int { b.len() }

struct Msg {
  body: Bytes
}

fn netlink(raw: Raw) -> List[Msg] {
  let buf = raw.bytes_buf(64)
  var out: List[Msg] = []
  for _ in 0..3 {
    let n = recv(buf)
    out.push(Msg{body: buf[0..n]})
  }
  out
}

fn resolver(raw: Raw) -> List[Bytes] {
  let buf = raw.bytes_buf(64)
  var addrs: List[Bytes] = []
  for _ in 0..2 {
    let n = recv(buf)
    let a = buf[2..n]
    addrs.push(a)
  }
  addrs
}

fn cloned(raw: Raw) -> List[Bytes] {
  let buf = raw.bytes_buf(64)
  var out: List[Bytes] = []
  for _ in 0..2 {
    let n = recv(buf)
    out.push(buf[0..n].clone())
  }
  out
}

fn inside(raw: Raw) -> Int {
  let buf = raw.bytes_buf(64)
  var total = 0
  for _ in 0..2 {
    let n = recv(buf)
    let v = buf[0..n]
    total += v.len()
  }
  total
}

fn no_read(raw: Raw) -> List[Bytes] {
  let buf = raw.bytes_buf(64)
  var out: List[Bytes] = []
  for i in 0..2 {
    out.push(buf[i..i + 1])
  }
  out
}

fn answer(m: Bytes, n: Int) -> (Int, List[Bytes]) {
  var addrs: List[Bytes] = []
  addrs.push(m[0..n])
  (n, addrs)
}

fn dns(raw: Raw) -> List[List[Bytes]] {
  let buf = raw.bytes_buf(64)
  var out: List[List[Bytes]] = [[], []]
  for i in 0..2 {
    let r = recv(buf)
    let m = buf[0..r]
    let (_, addrs) = answer(m, 4)
    out[i] = addrs
  }
  out
}

fn main(os: Os) {
  let r = Raw{}
  os.stdio.println("${dns(r).len()}")
  os.stdio.println("${netlink(r).len()} ${resolver(r).len()} ${cloned(r).len()} ${inside(r)} ${no_read(r).len()}")
}
`
	got := vetCodes(t, src)
	want := []int{16, 26, 74}
	if len(got["XO0541"]) != len(want) || got["XO0541"][0] != want[0] || got["XO0541"][1] != want[1] || got["XO0541"][2] != want[2] {
		t.Errorf("XO0541 at %v, want %v: %v", got["XO0541"], want, got)
	}
}
