kind: review
module: uuid
result: pass
commit: 2f2582761
source: c3cacf57c4b2235e9fff8cf0544f04a5c6fba12342c82c322019488bc35be316
date: 2026-11-14
host: darwin/arm64 (reading and tests), linux/arm64 host 2 (fuzz, stress, race), host 1 (perf)
command: none (section 3 does not apply: no secret is compared); read against knowledgebase/infrastructure/std-security-checklist.md
measure: 0 Go vulnerability entries for github.com/google/uuid (none in the database; 182 stdlib entries read, 2026-11-14); 2 inputs a peer controls (text to parse, bytes to from_bytes), each refused by its length before any other work; 1 defect found and fixed (F1: parse faulted on a 45 byte text with multi-byte text in its first 9 bytes, 9c3375e32, to ship in 0.51.2); 4 fault sites, none reachable from input; 0 insecure defaults (v4 and v7 draw from the operating system's generator)

Security review of std/uuid (std/uuid.xo, its primitives in
std/native/internal.xo uuid_word and uuid_half, and on the Go backend
std/rt/internal_lz4.go Internal_uuid_half) against
knowledgebase/infrastructure/std-security-checklist.md (decision 0616),
by Xo Lang 3. std/uuid follows github.com/google/uuid v1.6.0 (decision
0282): v4 and v7 generation, parse of the four text forms, from_bytes,
display and order. It reads text a peer sends: an id in a path, a
header, or a JSON body.

## 1. Go's CVE history

The Go vulnerability database (vuln.go.dev, searched 2026-11-14) has no
entry for github.com/google/uuid, and none of its 182 stdlib entries
covers UUIDs.

Sibling classes, checked:

- Predictable identifiers (the class behind session ids made from a
  weak generator): v4 draws 16 bytes and v7 10 bytes from the program's
  Rand capability, which spec/std.md 3.4 makes the operating system's
  generator (getentropy, getrandom) for a real program; only a test's
  seeded Rand is deterministic. google/uuid uses crypto/rand the same
  way. v7's 74 random bits follow the millisecond with no counter
  (uuid.xo:2), so two v7 ids in one millisecond do not reveal an order
  or a count.
- Error text built from input (the class F1 below is in): the one error
  that quotes the input quotes the first 9 bytes as Bytes, with Go's
  `%q` rules for a cut UTF-8 sequence, so it neither faults nor emits
  text that is not UTF-8.

## 2. Limits on hostile input

| Input a peer controls | Limit | Where | Test at and past it |
|---|---|---|---|
| Text to parse | the length first: 32, 36, 38 or 45 bytes, any other is refused with its length and nothing else read | uuid.xo parse (90-107) | fuzz TestUuidFuzz (every accepted length against every kind of byte at every position, against google/uuid); corpus testdata/corpus/uuid; std/uuid_test.xo |
| The hex digits and dashes of an accepted length | uuid_half checks the length against off before it reads, a 256 entry table maps each byte (0xff for a non-digit) | std/native/internal.xo uuid_half (699-728), UHEXVAL (656, 256 entries) | fuzz above; stress std/evidence/uuid/stress (64 texts, half near a valid form: cut, recased, multi-byte) |
| Bytes to from_bytes | exactly 16, else an error with the length | uuid.xo from_bytes (111-116) | fuzz above (0 to 32 bytes) |

Every input is at most 45 bytes once its length is checked, so time and
memory are constant per call.

## 3. Constant time

Not applicable: std/uuid compares no secret. A UUID used as a bearer
token is compared by the program, not by std/uuid.

## 4. Fault behaviour

### F1 (fixed in 9c3375e32, to ship in 0.51.2): parse faulted on multi-byte text in a urn prefix

For a 45 byte text whose first 9 bytes were not `urn:uuid:`, parse built
its error by slicing the input as a Str at byte 9. When byte 9 fell
inside a multi-byte UTF-8 sequence, the slice faulted, so a program
parsing a peer's UUID text stopped (an http handler answered 500 rather
than its 400). 0.51.0 and earlier, native and LLVM backends. Found by
the new fuzz harness on its first seed.

Fix: the error quotes the first 9 bytes as Bytes (`s[0..9].quote()`, uuid.xo:97),
which renders a cut sequence as `\xHH`, google/uuid's `%q` text. Tests:
std/uuid_test.xo "a 45 byte text with multi-byte text in its first 9
bytes is google/uuid's error, not a fault"; TestUuidFuzz on arm64, llvm
and go; std/changes/uuid.md.

### Fault sites

- uuid_word (internal.xo:724-729) faults when fewer than 8 bytes follow
  i. Its callers pass b from from_bytes after its 16 byte check, or
  r.bytes(16) and r.bytes(10) at offsets 0, 8 and 2; not reachable from
  input.
- urn_at (uuid.xo:67-79) indexes s[0..9] only when parse saw 45 bytes
  (uuid.xo:95); not reachable.
- uuid_half reads through raw addresses only after `s.len() - off >= n`
  (36 or 32), and for half 0 and half 1 the largest offset it reads is
  n - 1 (35 dashed, 31 undashed); the table read is a zero extended byte
  (rt.xo:277) into 256 entries.
- uuid_half does not check `half` (internal.xo:731-760): a half other
  than 0 or 1 would read past the text natively and panic on the Go
  backend (internal_lz4.go:293). Its only callers pass 0 and 1
  (uuid.xo:83-84), and it is internal to std, so it is not reachable.
  The two backends also differ inside the primitive (the Go backend
  checks all 32 digits for either half, native only its own half's), but
  decode always asks for both halves, so parse answers the same on both.

## 5. Defaults

- v4 and v7 use the program's Rand; a real program's is the operating
  system's generator. No option selects a weaker one.
- parse accepts google/uuid's four forms and, as google/uuid, does not
  check the braces of the 38 byte form (uuid.xo:103); this accepts
  no more than a 36 byte id with one byte on each side and changes no
  decoded bit.

## Sign-off

Second reader: a subagent of Xo Lang 3 (2026-11-14), reading the code
separately: needs changes (uuid_half's unchecked half and the two
backends' differences inside it, the fault site count, the release
wording, line numbers), all made above; it confirmed uuid_half's offsets
for both halves, the zero extended table read, UHEXVAL's 256 entries and
the length checks.
