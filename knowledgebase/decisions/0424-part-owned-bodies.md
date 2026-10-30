# 0424: A split generation's parts own their functions' bodies; the module keeps shells

Status: Accepted (xo-lang-6, 2026-10-30; the lead's go on the plan 2026-10-29 23:30)

## Context

The native back end's split generation (arm64/parallel.xo generate_cached) hands the whole
ssa.Module to every emit worker, and the module keeps every function's body (blocks, values,
block parameters, the inliner's copies, cse tables) until the Program returns: 2.3 to 3.5 GB of
the self build's heap are live through emit (heap snapshots past inline.verify and past emit,
2026-10-29), under the build's peak, while a part reads only its own functions' bodies and, of the
other functions, their names, signature facts and flags.

## Decision

1. generate_cached (arm64/parallel.xo) takes the module as `var m`: the driver's module is the one
   whose bodies move, and the driver (compiler/xo main.xo) reads what the link needs of it (debug
   information flags, comp_dir, std_root, std_obj, sepcomp_program) before the call, so the call is
   the module's last use and no copy of the functions list keeps the bodies.
2. After the address-taken scan and the part lists, each part's functions move out of m.funcs into
   a Mutex cell of that part (Map[Int, Func]); the module keeps `Func.shell()` of each (name, file,
   results, var_params, indirect, captures, entry, export, catches, unwind_exit, borrowed, skip,
   ckey, target, reader: the shape rccache.xo lazy_shell gives a function before its body is
   decoded) with its body facts recorded first (ssa.body_facts into m.facts, when absent): the
   module's readers of a function's parameters, names and faults (param_types, block_names,
   value_names, fault_facts, value_count) take the facts and never reach for the body.
3. The worker that takes a part takes its bodies out of the cell (take_bodies leaves it empty), the
   emitter reads its own function's body from G.bodies first (begin_func), and the bodies drop with
   the generator as gen_part returns; a part served from the part cache drops them at once.
4. The whole-program generator made before the move (the entry and the exports) takes the
   hollowed module (g.m = m) so its copy keeps no body either.

## Consequences

- Self build, cache off (A main's xo 0.34.0 at e0295d2e, B this change, probe 2026-10-30 00:11 on
  a loaded Mac; the --ab in a measure window gives the wall figures): heap past emit 9452 to
  8180 MB, heap peak 10764 to 9837 MB, footprint 7667 to 7287 MB, max RSS 9.75 to 8.86 GB; the heap
  in use past emit no longer holds ssa.Func.add_param (750 MB), arm64.inline_call (1.3 GB cum),
  ssa.renumber (378 MB) or ssa.cse (184 MB). Self build --ab (quiet measure window 2026-10-30
  00:35, 6 rounds, cache off, A main's xo at e0295d2e, B this change on the pre-F2 base): wall
  1.011 (1.002 to 1.015; 39.6 to 40.0 s), CPU 1.011, instructions 1.002, cycles 1.011 (IPC 2.67 to
  2.64), max RSS 0.954 (9.39 to 8.96 GB): about 1% of the self build's time for 0.9 GB of peak.
- Codegen identity against main's xo (interp, strings, darwin and linux arm64): equal; the cache-on
  fixed point (cold, functions hit, parts hit) is one binary; the arm64 goldens (431) pass but
  http_wire and http_stream, which fail on main with main's xo too (others' red).
- A function's body is read by its own part only: a later pass that wants another function's
  body during emit must take it from facts or run before the move (gotchas/module-shells-need-facts.md).
- The lazy functions (m.lazy, decision 0283 B) are untouched: their shells already have facts.

## Alternatives

- Freeing bodies after each gen_part by mutating the shared module from the worker: a copy on
  write of the 60K record list per part, and the parent's list keeps every body.
- Dropping the whole module after fn_keys on warm builds only: the cold build, the peak that
  matters, keeps everything.
- Streaming SSA to disk per part (as 0418 does for cache entries): the bodies are needed once and
  are dropped here; nothing to read back.
