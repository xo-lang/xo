# cc object cache keyed on the build's random directory

Status: Observed (2026-11-05, linux/arm64 host 1, xo-lang-4)

## Symptom

A warm xobuild on Linux reused 119 of 125 parts but 0 of 126 objects
(`cache objects 0/126 reused`), and assembling took 9.2 of the warm
build's 15.9 s. A Mac build and a direct `xo build` on Linux did reuse
objects (117/126 after an llrt edit).

## Cause

On Linux xobuild sets XO_NOLIBC=0, so the build links with cc and
assembles its parts with cc (compiler/xo/main.xo, cached_cc_assemble).
That cache keyed each object on its .s file's full path, because an ELF
object names its source. The path was in the build's private directory
`w-<random hex>` (main.xo), so every key was new. Xo's own assembler
path keys on the part's text only and was not affected.

## Fix

cc runs in the part's directory on the base names, and the key takes
the base name (asmparts.xo cc_assemble, cached_cc_assemble), as
split.xo already did for clang's LTO modules
(gotchas/windows-thinlto-module-path.md). After the fix the same warm
edit reused 123 of 126 objects, and the warm build went from 15.7 to 15.9 s down to
9.8 to 10.1 s (assembling 9.2 s down to 3.2 s, host 1, 2026-11-05).

Then (same day) an object is keyed by its part's back-end cache key
(arm64 partcache part_keys) with the xo identity, the DWARF slots, the
subsections flag and, for a part hollowed after keying, its live
functions, so a held object needs no text: only the parts cc assembles
get a .s file (cc_obj_keys, cc_obj_held). A build that hollows without
the back-end cache hashes the texts as before. Warm edit build 9.4 to
10.3 s, assembling about 1.2 s.

## How to avoid

A cache key must never take a path under a per-build random directory.
Key on what the tool records (a base name), and run the tool where that
name resolves. See also output-from-cache-keys-differs-cache-off.md.
