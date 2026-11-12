# std security review checklist (decision 0616)

A module's `review` evidence (`std/evidence/MODULE/review.md`) is an
agent-written security review that answers every item below for the
module, or for the part of it the review names. Required for crypto, tls,
http and x509, and for any module decision 0461 lists as parsing untrusted
input that opts in. Last verified: 2026-11-12.

The review's header is decision 0616's (`kind: review`, `module`,
`result: pass`, `commit`, `source`, `date`, `host` for the timing runs,
`command` for the timing test, `measure` summing the items). Its body
has one section per item, then the sign-off.

## 1. Go's CVE history

For the Go package the module follows, every entry in the Go
vulnerability database for its import path (`https://pkg.go.dev/vuln/`,
search the import path; also the `golang.org/x/...` path it ports, if
any). For each entry:

- the entry's ID and a one line summary;
- whether Xo's code shares the defect, and why (the code it ported, or a
  different design);
- a test that reproduces the entry's input against Xo and shows Xo's
  answer (a golden, a module test, or a fuzz corpus row), named by path.

An entry about a part of Go's package Xo does not have says so in one
line, with no test.

## 2. Limits on hostile input

Every length, count, depth and time a peer controls (header bytes, header
count, nesting depth, decompression ratio, handshake messages,
certificate chain length, ciphertext length), and for each:

- the limit applied and where (file:line);
- a test at the limit and one past it.

## 3. Constant time (crypto, tls)

Each operation on a secret (MAC and tag compares, padding checks, scalar
multiplication, RSA private operations, AEAD tag checks):

- how it avoids branches and memory indexes that depend on the secret,
  from reading the code (std/native/llrt/ct.xo's rules: masks, ct.pick,
  ct.barrier, one branch on the final public answer);
- a timing test, dudect style: two classes of input (fixed and random, or
  valid and invalid), at least 10^6 measurements per operation,
  interleaved at random, Welch's t-test over the cropped measurements;
  pass when |t| < 4.5. Runs on host 1 as an exclusive job (no other load
  on the machine), on the native backend and LLVM.

## 4. Fault behaviour

No input a peer sends makes the module fault (on a server a fault ends
the program). List each fault site reachable from input (`or_fault`,
indexing, `!` unwraps, arithmetic that can overflow) with the error the
module returns instead, or why the site cannot be reached from input.

## 5. Defaults

Insecure options are off by default (TLS versions and cipher suites,
redirects across schemes, cookie scope, legacy schemes such as PKCS #1
v1.5 encryption), and each insecure option a caller can turn on is named
in its doc comment as such.

## Sign-off

A second agent, which did not write the review, reads it against the
code and adds a last section `## Sign-off` with its name, the date, the
commit it read, and each finding it raised with how it was resolved.
Findings become fixes (with tests) before the review is committed. A
review goes stale with the module's source hash (decision 0616 section 3)
and is redone when the module changes in a security relevant file.
