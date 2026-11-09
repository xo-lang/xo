# Intel loop timings follow the loop's placement, not only its code

Status: Observed (2026-11-08, Xo Lang 4; decisions 0577 and 0579).

## Symptom

A change that removes work from a hot x86 loop measures slower on the
Intel laptop (i7-9750H, Coffee Lake) while Zen 5 and arm64 show no
change. Decision 0577 dropped a never-taken `jo` from bytesum's byte
loop (5 uops to 4) and the program ran 27% slower there (404 to 511 ms).

## Cause

Not the JCC erratum (the assembler already pads jumps, decision 0398,
and the loop sat inside one 32-byte window), not the `lea` the add
became (a hand-patched `add` was slower still), and not the missing
branch (a 6-byte NOP in its place ran 389 ms, a fused `jb` 316 ms). The
same four uop loop ran 316 or 508 ms depending only on where its bytes
and its outer loop's bytes fell: Xo aligns x86 functions to 4 bytes
and loop heads not at all, so any change before a loop moves it.

## Fix / how to avoid

- Before blaming or crediting a codegen change on Intel, rebuild with
  `XO_PASSES=+x86_loop_align32` on both sides (decision 0579's opt-in
  pass; it aligns every loop head to 32 bytes): the 0577 loop then ran
  327 ms, a 19% gain where the unaligned build showed a 27% loss.
- Hand-patch the binary (same length, `codesign -s - -f` on darwin) to
  split a code effect from a layout effect; scripts in
  ~/xo-lead/notes/xo-lang-4/scripts (leaadd-intel2.sh).
- Zen 5 and Apple M are far less sensitive; a claim measured only there
  can still flip on Intel.
