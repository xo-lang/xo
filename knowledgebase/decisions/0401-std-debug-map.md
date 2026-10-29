# 0401: a debug map for separately compiled darwin links

Status: Accepted (owner approved option 2, 2026-10-28; pushed on the owner's decision of 2026-10-29 as a smaller binary and a debugger that sees std, not as a faster link)

## Context
A separately compiled darwin link (decision 0388) copied std's DWARF into every edit
binary as segment __DWARF: about 11 ms of the HTTP server's link (the copy plus the
UUID and code signature hashes) and 0.7 MB of a small program. Xo's own tracebacks read
only lines and inline frames (dwline.xo); variables only under XO_TRACEBACK=locals and
in debuggers.

## Decision
- The link writes the runtime's tables as segment __XODW, sections __xo_line, __xo_info,
  __xo_abbrev, __xo_str, __xo_line_str, __xo_aranges (and the program's __xo_loclists,
  __xo_addr), no S_ATTR_DEBUG; std's location lists and addresses are left out.
- A Mach-O debug map (N_SO, N_OSO with time 0, N_BNSYM/N_FUN/N_ENSYM, N_STSYM) names
  std's part objects (kept beside the store as <store>.parts, marked used on reuse) and
  the program's objects (copied once to <backend cache>/dbgobj/<sha256>.o, marked used
  by each link that names them). lldb and dsymutil read full DWARF from them.
- dwline.xo (d6ca7b43c) and compiler/debug read __XODW; xo dbg and xo dev symbolize
  unchanged.

## Consequences
- An edit binary has no __DWARF; lldb sets std breakpoints, shows bt with lines and
  variables through the map; dsymutil on the output gives a full dSYM.
- A binary copied elsewhere after a prune keeps Xo tracebacks with lines; lldb shows no
  variables for objects it cannot find.
- Two cache dirs give equal bytes apart from the N_OSO paths.

## Proof (2026-10-29, darwin/arm64)
- lldb on an edit binary: `breakpoint set -n prelude.str_split` stops at prelude.xo:101,
  `bt` names the program's main.xo:2 (its kept object), `frame variable` prints std's
  Str arguments.
- dsymutil on the output: a dSYM of 1380 subprograms with std's location lists.
- xo dev unwind and xo dev debug: frames with lines; with the dSYM and --locals, std's
  variables. xo dev symbolize: a stack overflow report's 65 line traceback equal.
- XO_TRACEBACK=locals: the same variables as a whole build and as the parent's xo.
- A binary copied away with its cache gone: Xo's traceback with lines; lldb's std
  breakpoint pending (no failure).
- Two cache dirs: equal bytes but LC_UUID (decision 0400's UUID hashes __LINKEDIT, which
  holds the N_OSO paths); the parent's xo gives equal bytes.
- Xo's objects write a DWARF pointer as its addend under an extern relocation; lldb reads
  a mapped object's DWARF without relocating it, so the map's copies are rewritten as
  clang writes them (dbgobj.xo; gotchas/debug-map-object-addresses.md).

## Measured
- Measured (quiet interleaved A/B, n=80, the separately compiled store link, A the parent's
  xo, 2026-10-29): fib 0.981 (finish 3.59 to 3.33 ms), the HTTP server 1.004 (finish 15.1 to
  14.6 ms, prepare 1.048: the stabs). Not a link speedup: the runtime's lines and info, which
  tracebacks need, are still copied (__XODW); only std's location lists and addresses leave.
  The gain is the binary (a small program 2.03 to 1.32 MB) and a debugger that sees std's
  variables through the map. A later step: std's __XODW tables precomputed in the store (one
  copy instead of the per-unit cut) or read in place (item 2 of the link plan).

## Alternatives
- Embed std's DWARF (before): the cost above.
- Embed only lines and map the rest: lldb does not combine embedded DWARF with a debug
  map; any __DWARF segment and it ignores the map (verified 2026-10-28: std breakpoints
  unresolved). Hence the __XODW names.
- A shared DWARF cache entry per std build: hit but no gain (copy still needed).
