kind: review
module: fnv
result: pass
commit: 53ab9cb30
source: 5f34cd0ac2005b37cf77ebf324b2f8d539b5827efe52c87aebbb39fc17218cba
date: 2026-11-14
host: darwin/arm64 (reading and tests), linux/arm64 host 2 (fuzz, stress, race), host 1 (perf)
command: none (section 3 does not apply: fnv is not a keyed or secret hash); read against knowledgebase/infrastructure/std-security-checklist.md
measure: 0 Go vulnerability entries for hash/fnv (182 stdlib entries read, 2026-11-14); 3 inputs a peer may control (the bytes hashed, linear time and constant memory with no limit, as Go; a 32 or 64 bit sum passed back, any value accepted; a 128 bit sum passed back, exactly 16 bytes or a fault, tested at and past it); 0 defects found; 1 fault site (update128 and update128a with a sum that is not 16 bytes, documented); 0 insecure defaults

Security review of std/fnv (std/fnv.xo; the Go backend runs the same Xo
code) against knowledgebase/infrastructure/std-security-checklist.md
(decision 0616), by Xo Lang 3. std/fnv is Go's hash/fnv: FNV-1 and
FNV-1a at 32, 64 and 128 bits (spec/std.md 45). It is pure: no I/O, no
allocation but the 16 byte result of the 128 bit forms.

## 1. Go's CVE history

The Go vulnerability database (vuln.go.dev, the 182 stdlib entries
fetched on 2026-11-14) has no entry for hash/fnv, and none for any
hash/* package.

Sibling class, checked:

- Hash flooding: FNV is not keyed, so a peer who chooses the keys of a
  hash table indexed by an FNV sum can choose colliding keys. This is a
  property of the algorithm, the same in Go. std/fnv's module comment
  says it is not for anything an attacker chooses the input of
  (fnv.xo:3-5). No std module or compiler code uses std/fnv (searched
  std/ and compiler/ on 2026-11-14), and Xo's Map does not hash with
  it, so no std surface exposes the class.

## 2. Limits on hostile input

| Input a peer may control | Limit | Where | Test |
|---|---|---|---|
| The bytes hashed (every sum and update form) | none, as Go: one pass, a multiply and an xor per byte, constant memory | fnv.xo update32 to update64a (37-69), step128 (104-121) | stress program std/evidence/fnv/stress (64 tasks, the six sums and update32a, update64 and update128a from a moving split, over 64 random inputs of up to 2 KB against Go's sums, resident set sampled; the fuzz below covers all six update forms); the loops have no data dependent branch, so time is linear by construction; fuzz TestFnvFuzz (0 to 600 bytes, every byte value, runs of 0x00 and 0xff) |
| A 32 or 64 bit sum passed back to update32, update32a, update64 or update64a | none needed: any Int is a state, and update32 and update32a mask the result to 32 bits (fnv.xo:44, 52) | fnv.xo:37-69 | fuzz above (every update form from a split) |
| A 128 bit sum passed back to update128 or update128a | exactly 16 bytes, else a fault | fnv.xo check16 (123-127) | golden internal/llvm/testdata/golden/fnv_caller_loc (a 16 byte sum continues, a 10 byte sum faults at the caller's line), run on every backend's goldens |

Nothing in the module allocates from a size it reads: the 32 and 64 bit
forms return an Int, the 128 bit forms build 16 bytes.

## 3. Constant time

Not applicable: FNV is not a keyed hash and std/fnv handles no secret.
Its time depends only on the input's length.

## 4. Fault behaviour

No defects found. The fuzz harness (TestFnvFuzz, corpus
internal/stdtest/testdata/corpus/fnvfuzz) compares every sum and every
update split against Go's hash/fnv on arm64, llvm and go.

### Fault sites

- check16 (fnv.xo:123-127): update128 and update128a fault, at the
  caller's location, when the sum passed in is not 16 bytes. The doc
  comment states it (fnv.xo:71). The review assumes a sum is a value the
  program computed with sum128 or update128. A program that resumes a
  hash from state a peer stored can reach the fault, and std/fnv offers
  no form that returns an error, so such a program must check the
  length first, as for any slice it indexes; Go's hash has no
  counterpart taking a sum (its state is restored through
  UnmarshalBinary, which returns an error). word_be (129-135) reads
  bytes 0 to 15 only after check16, so it cannot index out of range.
- Arithmetic wraps by design (wrapping_mul, wrapping_add, wrapping_shl),
  Go's uint64 arithmetic; no checked operation can overflow.

## 5. Defaults

No options and no defaults.

## Sign-off

Second reader: a subagent of Xo Lang 3 (2026-11-14), reading the code
separately: needs changes (the stress program's forms, a test at check16's
limit, the input count, the resumed sum assumption, the Int sums as an
input), all made above; it confirmed every line cited and that no std or
compiler code uses std/fnv.
