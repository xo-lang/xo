# 0428: An edit build parses only the std declarations it needs

Status: Accepted (owner, 2026-10-30; proposed by xo-lang-4 the same day,
the edit-build loop item; shipped in a7130ba9d)

## Context

An edit rebuild of the HTTP server benchmark (bench/perf/http/xo: 4 files,
382 lines, one module) on the arm64 offload host spends `check` 27.3 ms of a
~97 ms build (--phase-times, a function added to api.xo; the link from std's
half since decision 0420). Twenty such builds under XO_PROF=cpu, merged with
pprof (2026-10-30):

- parse_prefetch's lexing and parsing: about 14 ms of CPU a build
  (lexer.lex 0.18 s and the parser 0.10 s of the twenty builds' 0.28 s);
- check_program: about 12.5 ms (check_dirs 9.5: install_std 5,
  relocate_export 3);
- checking the program's bodies: too small to show.

The parse list (bcache.xo parse_list_store) names 16 files: the program's 3,
and 13 std files, 447 KB. Ten of the std files are parsed whole ("f", a lazy
parse whose bodies are skipped but whose every token is lexed), among them
std/native/prelude.xo (138 KB) and std/native/http.xo (120 KB).

Why they are parsed although std's checked interface comes from its record
(decision 0380, decision 0388 stage 2): check/dump.xo, with trees_lazy, calls
ensure_tree for the module of every std generic declaration, since the SSA
builder instantiates std generics inside bodies the check never visits; and
need_tree (a constant's initializer, a default's presence, a forced lazy
body, a std type's declaration) calls ensure_tree, which parses every file of
the declaration's module. A top-level scan of the seven native files
parsed whole: 334,881 bytes, of which the generic declarations are 38,387
(11%; native/prelude.xo 70 of them, 28 KB of 138 KB; native/http.xo 16, 4 KB
of 120 KB).

The check's wall, split with check.* sub-laps (--phase-times; three edits
on the arm64 host, 2026-10-30): check.setup 5.5 to 5.8 ms (the options,
std's entry read and decoded, load_std_record), check.prefetch 6.1 to 6.4
(parse_prefetch: the 16 files parsed on tasks, about 14 ms of CPU, so its
wall is the longest files' parse), check.dirs 14.1 to 14.9 (the checker's
run: install_std about 5 and relocate_export about 3 of its CPU), of 26.4
to 26.8. The parse is on the path (the check waits for the prefetch), but
it is a quarter of the check's wall: this decision's bound is about 6 ms of
wall, check.dirs is the larger lever, and decision 0380's stage 2 result
(cached parses, about 10 ms) agrees in size. Lang 1's earlier spike and
stage 2 branch (refs/recover/lang1-0380-stage2) are the alternative below.

## Decision

1. **Ranges in the record.** Each StdFileRec keeps its file's
   File.ranges (parser: each top-level item's start, its doc comment's if
   earlier, the offset after its last token, and how many declarations it
   made), none when the file had diagnostics; EXPORT_VERSION v4, so an
   older entry is a clean miss. A build's parse_file_ast drops a test's
   range with the test (skip_tests), so a range's indexes stay those of
   ast.decls and of the records' decl_idx.
2. **A declaration parse.** parser.parse_decl(name, src, range, sig, lazy)
   lexes only the range (lexer.lex_from) and parses its declarations,
   spans in file offsets, in the mode ensure_tree's parse would use
   (parse_lazy_path); None when they do not parse alone as counted.
3. **One gate, no pending entries.** A record file keeps today's
   invariant: unloaded, with ast.decls empty, until ensure_tree parses it
   whole. ensure_decl(f, i) parses the range holding declaration i into
   Checker.decl_parsed (file to index to tree) and leaves the file
   unloaded; top_decl(f, i) reads the tree or decl_parsed. need_tree and
   dump.xo's generic loop call ensure_decl for the object and its
   receiver; install_std leaves every object of an unloaded file pending; the
   generic ones are joined right after through need_tree, which marks a
   joined body for the lazy check (check_module walks no declaration of
   an unloaded file). A std struct literal joins the type's native twin
   too, since the SSA builder lowers the twin with its own defaults
   (http.Response names every field; native Response adds `stream`). The checker's readers that
   index a declaration (info.xo const_ref_compile_time, stmt.xo
   const_type, decl.xo's type tree, realtime.xo const_int, timelayout.xo
   const_str_lazy, and the SSA builder's invariant_of and const_value) go
   through top_decl; the readers that iterate
   ast.decls see an unloaded file's empty list, as before, so they need
   no change. ensure_decl does not enter Checker.parsed, so the next
   build's prefetch list leaves such files out. Audit:
   about 90 references of File.decls (xo refs), the indexed ones listed above.
4. **Off switch and proof.** XO_STD_DECL_PARSE=off parses whole modules as
   today. A parser test parses every declaration of every std file both ways
   and compares the trees (as the lazy parse's test does); the goldens on
   every backend, the fixed point, scripts/identity on the sample, and
   TestStdChecks.

## Consequences

- An HTTP edit parses about 38 KB of std's native sources instead of
  335 KB; the bound on the saving is about 9 ms of CPU of the 14. The
  check phase of an HTTP edit build on linux/arm64 went from about 27.5 to
  22.5 ms (3 parses instead of 16). xo bench --ab on the linux/arm64
  offload host, 150 rounds, the HTTP edit build from warm std caches,
  with A the parent's xo: wall 107.5 to 98.6 ms, 0.917 (0.847 to 0.976,
  better), and a repeat gave 108.0 to 99.4 ms, 0.92 (0.851 to 0.996,
  better). Both ran with a host load median of 7.6 and 0 loaded rounds.
- The record grows by two integers per declaration.
- A record file's ast.decls stays empty until ensure_tree, as before, so
  a reader that iterates it is unchanged; a reader added later that indexes
  a record file's declaration directly faults past the empty list, so it
  goes through top_decl (the indexed readers above fault with the file's
  path when a declaration is out of reach).

## Alternatives

- Cache std files' parse trees (an encoded AST keyed by content): it saves
  the lexing too but needs a full AST format and a decoder, and still
  reads every declaration of each file (not measured).
- A byte-level body skipper in the lexer (brace matching aware of strings,
  characters and comments) so a lazy parse never tokenizes a body: saves
  less (every declaration outside a body is still lexed) and touches the
  lexer every program uses.
- Keep whole-module parses and make them parallel: the CPU is spent anyway,
  and a build already runs other work on the other cores.
