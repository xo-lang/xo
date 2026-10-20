# Agent poll, 2026-10-20

Owner-requested poll of the six working sessions on writing Xo, two rounds: round 1 from habit (around 12:45), round 2 after each agent used the tools built since the 2026-10-18 survey on its real work (around 14:00). Ratings out of 10. Outcome: the owner approved the agent-tools fix-up (Xo Publish) the same day.

| Agent | Round 1 | Round 2 |
|---|---|---|
| Xo Lang 2 | 7 | 7 |
| Xo Lang 3 | 7 | 7 |
| Xo lang 4 | 7 | 7.5 |
| Xo lang 5 | 7 | 7 |
| Xo Lang Lib 1 | 7 | 7.5 |
| Xo Publish | 7 | 7.5 |

## Round 1 (habit)

    Xo Lang 2 (compiler, llrt, runtime): 7 (10-18 about 6).
    S: precise fixable diagnostics (XO0501 pointed at infer.xo:640); xo check ./xo in seconds before 5-min xobuild; match/enums/Option honest, formatter never surprises.
    W: failed self-build costs 5 min + slot (one-line receiver error); API discovery (guessed StrBuilder.write_byte, UInt8.to_int); test-layout tribal (gen goldens framing, explain_data rows found only by gate).
    Sug: xobuild runs xo check first; xo doc prelude Type / lsp completion from CLI; xo dev new-code / new-golden scaffolds.
    10-18 changes: none exercised this session (retracted earlier claims of 0261/0268/0258/0260).

    Xo Lang 3 (http, tls, caches): 7 (10-18 about 6). App-level pleasant; std/native+llrt raw-memory with sharp edges.
    S: teaching diagnostics (XO0177 try, XO0206 discarded Result); 3 backends from one source, tls_client agreed first run; XO_LEAKCHECK catches handle leaks.
    W: llrt handle plumbing undocumented convention (new_handle/obj_at/h_release/free fn must free_obj, cabi_* export by literal name; leaked one today, near name clash); native errors wrapped in XO1201 "not available without GC" when it's XO0605 not pub; integer overflow trap becomes silent 500 in a handler.
    Sug: plain check errors inside std/native (no XO1201 wrapper); llrt `handle` helper (counted handle + free hook); log handler fault message to stderr in debug builds.
    Helped: 0259, 0262, 0267, 0268. Unused: 0261 (grep), dbg, 0277.

    Xo lang 4 (arm64, llrt): 7 (10-18 about 6).
    S: teaching diagnostics (XO0601 capability fix, XO0502 "declare ys with var"); xo check fast enough per edit; std private tests 0262.
    W: unused bindings hard errors mid-edit (XO0110 calls.xo:156); no did-you-mean for missing method (Str index_of vs find, x86emit.xo:534); keyword as name error not explicit (`var in`).
    Sug: run linux/amd64 test programs on the Mac in -short (qemu-user or emulator behind xo test --target); did-you-mean for methods/fields; xo check --wip (unused as warnings).
    Used: 0262, 0260, 0268 (indirect). Unused: 0261 (grep habit), dbg, --watch, vet; 0258/0259 didn't come up.

    Xo lang 5 (compiler/ssa, xo, check): 7/10 (no 10-18 recall).
    S: uses fs.read makes I/O visible; --phase-times + XO_PROF + xo prof; verification culture (cache-off compare, verify mode, fp).
    W: value-semantics cost opaque (Checker captured by value feprebuild.xo:58; unsure if push copies list); optional file read verbose `(os.fs.read(p).ok() ?? b"").to_str() ?? ""`; edit loop 5.6 s + xobuild 74 s makes wrong attempts cost minutes.
    Sug: compile server/edit loop; documented COW rules + vet note for large struct copied into task; fs.read_str_or(path, "").
    Helped: --phase-times, XO_PROF, xo prof, hang sampler (0268 unverified, corrected). Unused: xo def/refs (grep habit), dbg, --watch, 0277.

    Lib 1 (std, services from spec): 7.
    S: diagnostics with fixes (Rust-name hints, did-you-mean); Result/!/? + capabilities make tests deterministic, Go tables port as data; one program on 4 backends, std module tests ~1 s.
    W: XO0510 move-only builders (freeze inside loop/early return forces restructuring: multipart read_part, qp_decode, uuid parse, base64 decode) costs most time; every std module written twice (native .xo + Go glue; std_gen.go boilerplate, rebase conflicts); interpolation needs Display (.debug() everywhere, XO0404 x12), prelude gaps (Str.rfind, Bytes zero-fill, min Int64 literal XO1201).
    Sug: generate std_gen.go + Go wrappers from std declarations; flow-sensitive moves; ${x:?} debug interpolation; raw TCP listeners in FakeNet.
    Spec wrong: std.md 8 os.tasks in native tests (faults); FakeNet raw dials; 10 Row Debug natively <row>; core 2.8 newtype JSON. Method tables incomplete (Str.find Option, Rand.bytes).

    Xo Publish (lsp, compiler/cli/servers): 7.
    S: fast precise checks (0.35 s; XO0701 names exact effect, XO0403 prints fix); XO_TRACE + xo trace found scheduler bug in ~20 min; tasks/channels easy, worked first try.
    W: await vs channel wait look equivalent but root-inline stalls 7 s, undocumented; `let when` gives "expected an expression, found ==" not "when is a keyword"; compiler edit loop 100 s build + fp + heavy queue + push queue (40+ min waits).
    Sug: push queue not serializing docs-only pushes behind overlap gates; fix root-inline (or XO_TRACE warning >100 ms); reserved-word diagnostic naming the keyword.
    Used none of 0258-0268 directly (text replacement instead of xo edit rename / def / refs, habit). Effect diagnostics and 0240 trace were the wins.

## Round 2 (after using the tools)

    Xo Lang 2 round 2: 7 (unchanged).
    Good: xo def (all 3 park_waits with docs, 1.6 s); xo explain; xo trace (found the 76 ms HTTP stall; best tool); xo prof; --phase-times; adr-claim.
    Failed: def with module-qualified llrt names ("no declaration named pool.park_wait"); xo refs missed cross-module call (tasks.xo:184) that grep finds; query callers WRONG answer (tasks.park_wait's caller for pool's); xo vet native/llrt -> "5909 check errors" (can't vet the runtime).
    Not used: edit, dbg, TRACEBACK locals, --std-module, --watch, symbols, query type/effects, doc.
    Sug: make def/refs/callers/vet module-aware for std/native/llrt (one module per file).

    Xo Lang 3 round 2: 7 (unchanged).
    Good: xo test --std-module (best: private test 1.4 s vs minutes for a golden; clear XO0601); xo edit rename --dry-run (8 edits, clean plan, better than sed); refs on program code (exact, 0.01 s); query callers/effects with --dir; explain; prof + --phase-times essential; dbg fine.
    Failed: xo def for std names (tls.client, tls.Verify, tlsio.client_with: no declaration); query type without --dir "not part of the checked modules" (should default to file's dir); callers needs full module path; symbols --dir=program returns [] (non-pub skipped); xo doc std/tls "No exported declarations" (std files have no pub) -> doc misses all of std (fix first); XO_TRACEBACK=locals <unavailable> + no inlined frame locals; xo vet std/native -> 6787 check errors.
    Sug: make def/doc/symbols understand std and llrt (no-pub declaration files, native/... paths).

    Xo lang 4 round 2: 7 -> 7.5.
    Good: xo def/refs (1.3 s, exact, beats grep; JSON by default noisy); query callers (beats grep for methods); query type; xo explain XO0110 (names xo fix); xo vet (1.7 s, real XO0532 hit at x86.xo:2656); dbg + vet XO0901.
    Failed: def/symbols don't index prelude methods ("no declaration named Str.find"; symbols --dir=std 6.8 s); xo doc needs a module not a symbol; xo edit rename correct but 13 s vs sed instant; XO_TRACEBACK=locals: FAULTING frame's locals <unavailable> on arm64 (div by zero in loop), only main's printed.
    Not used: --watch, prof/trace (callgrind), phase-times, adr-claim.
    Sug: amd64 emulator in -short; index prelude methods in def/symbols/doc; text output by default at a terminal.

    Xo lang 5 round 2: 7 (unchanged).
    Good: def/refs precise (2 exact uses) but 2.8-3.8 s vs grep instant; query effects standout (found `uname -s` ~11 calls per build in 1.7 s); query api shaped checker-cache key; vet + explain XO0531 explained Checker copy cost; prof + XO_PROF + phase-times best daily; --std-module via test found shared test-binary bug.
    Failed: --dir=. at workspace root "no .xo files"; effects rejects main.fn and full module path (bare name only); edit dry-run gives counts not a diff; xo doc --dir=. Checker -> 210 KB one-line JSON with absolute paths, no --text.
    Sug: --dir at workspace root; same name forms across all tools.

    Lib 1 round 2: 7 -> 7.5.
    Good: xo query def|callers|effects|type beat grep (query def answered "does Str.rfind exist" in 1 s; query def Str.find -> prelude.xo:211); edit rename dry-run (10 edits clean); dbg (no .debug() needed; vet XO0901) partly fixes the Display complaint; explain XO0510.
    Failed: `xo def`/`xo refs` SHORTCUTS fail on std/prelude names while `xo query def` works (bug); wrong XO_STD silently gives not found/<error> (want "program does not check" note); edit refuses on non-checking program (right but surprising); XO_TRACEBACK=locals all <unavailable> (index fault in main); xo vet takes a dir only, no --std-module.
    Sug: fix def/refs shortcuts; xo vet --std-module; locals for plain lets.

    Xo Publish round 2: 7 -> 7.5 (query/edit/vet/dbg loop real time saved).
    Good: def/refs (3 exact sites 0.6 s, better than grep which hits comments); query type/callers/effects beat reading code; edit rename real use (2 edits, clean); vet found XO0532 + flagged dbg leftovers XO0901; dbg confirmed dependent recheck; explain; --phase-times.
    Failed: module-qualified names as written in code (`xo def check.Checker.check_dirs`, `xo doc check.new_checker`) "no declaration"; symbols --dir lists 1289 imported public names, not the dir's own private decls; `xo query --text effects X` rejects flag order; rename doesn't support types.
    BUG: xo build --phase-times ./compiler/xo: "front end cache's entry for ssa.Builder.unbox_type does not replay" (XO_FE_CACHE=off builds) -> sent to Xo lang 5.
    Sug: symbols --dir = own decls incl private; def/doc accept module.name as in code; rename types.
