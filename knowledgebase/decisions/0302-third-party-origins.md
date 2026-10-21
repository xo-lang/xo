# 0302: Third-party origins marked in the source and checked against THIRD_PARTY_NOTICES

Status: Proposed (Xo Publish, 2026-10-21; for the owner's decision, no
implementation before it). THIRD_PARTY_NOTICES is legal text: this
decision only proposes how the tree keeps it complete; the notices'
wording stays with the owner.

## Context

The release archives (scripts/release.sh, decision 0157) ship bin/xo,
bin/xo-gobind, the tracked files of std/, lib/xo/race, and
THIRD_PARTY_NOTICES. A read-only license audit of those contents on
2026-10-21 (origin/main 18432657) found that the notices covered Go and
golang.org/x/crypto, LLVM compiler-rt, google/uuid, and pgx, and missed:

- bin/xo-gobind, a Go binary: it links Go's runtime and standard
  library, which Go's license asks to be noticed in a binary
  distribution; the notices named only Go code ported into std/.
- fmod in std/native/llrt/libcmath.xo, a step by step port of musl's
  src/math/fmod.c (MIT), compiled into every program linked with no C
  library (decision 0243) and into the Linux xo. Its comment said
  "musl's algorithm" and nothing tied that to the notices.
- Lower risk: glibc 2.34's strerror texts in std/native/llrt/libcstr.xo,
  the IANA time zone database in std/tzdata (public domain), the Unicode
  data behind the Unicode tables, and the RFC 7541 HPACK tables.

The cause is that origins are free text in comments ("follows Go's
...", "musl's algorithm", "glibc's texts"), which nothing can check:
"port of" in this repository mostly means an internal Go-to-Xo port
(internal/parser, internal/lexer) or a network port. A survey of
origin/main finds outside projects named in comments 65 times (glibc
26, x/crypto 17, google/uuid 14, pgx 5, musl 3), most of them
references rather than copies (glibc's struct dirent layout, errno
names).

## Decision (proposed)

1. A file (or function) that copies, ports, or embeds code or data from
   outside the repository says so with a marker line:

       // Origin: musl src/math/fmod.c (fmod)

   The first word is a project key; the rest names what was followed.
   Following a library's behavior through a differential test, with no
   code or data copied, needs no marker.

2. Each THIRD_PARTY_NOTICES item carries its keys on its heading line
   (`5. musl libc [musl]`). An item for something the release script
   ships rather than the tree (compiler-rt, xo-gobind) is marked
   `[binary]`.

3. internal/repotest TestThirdPartyOrigins, in -short (it reads tracked
   files only):
   - every `Origin:` key in compiler/, std/, scripts/, and cmd/ names a
     notices item;
   - every notices item is named by an `Origin:` marker or is `[binary]`;
   - a shipped file whose comments name a listed outside project (musl,
     glibc, bionic, FreeBSD, OpenBSD, fdlibm, Cephes, BoringSSL,
     Wycheproof, Mozilla, zlib, SQLite) without an `Origin:` marker
     fails, unless the line is in
     internal/repotest/testdata/origins-allow.txt, the reviewed list of
     references that are not copies.

4. scripts/release.sh checks that the license file of every `[binary]`
   item it stages exists in the staged tree (lib/xo/race/LICENSE.TXT
   beside a race runtime).

5. Go stays one blanket item (`[go]`): the files ported from Go already
   say which Go code they follow, and marking about a hundred of them
   adds no coverage. Only non-Go origins get markers. (The alternative,
   one marker per Go-ported file, is listed below.)

## Consequences

- An origin added without a notice fails -short at the commit that adds
  it, not at a release audit.
- The allowlist starts as the reviewed current mentions (the glibc
  layout and errno references, musl's errno name) and grows only by
  review.
- Today's gaps need the owner's wording before the test can pass: the
  audit's draft (release-archives branch, a local commit) adds the
  xo-gobind line, a musl item with musl's MIT text, and an IANA tzdata
  item; glibc's texts, the Unicode data, and the RFC 7541 tables need a
  call (an item each, or an allowlist line saying why none is needed).
- Cost: one test of about 100 ms in -short, and a marker line in a
  handful of files.

## Alternatives

- Keep auditing by hand before each release: the 2026-10-21 audit is
  what found the gaps, but nothing stops new ones between audits.
- A marker on every Go-ported file too: complete, about a hundred files
  to touch, and no gap the audit found would have been caught sooner.
- An SPDX header on every file: the standard form, but it describes a
  file's own license, not the origin of code inside it, and the
  repository's own files are MIT (LICENSE) and MIT-0 (std/LICENSE)
  already.
- A license scanner (scancode, licensee) in CI: it finds license texts,
  not a port whose comment says only "musl's algorithm", and CI is
  started by hand (decision 0225).
