# 0428: An edit build parses only the std declarations it needs

Status: Proposed (xo-lang-4, 2026-10-30; the edit-build loop item, lead
2026-10-30: the parse cache is this claim's; held until the 0.35 seed is cut)

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

## Decision (proposed)

1. **Ranges in the record.** std's export record keeps, for each file, the
   byte range of each top-level declaration (start of its doc comment or
   attribute, end of its last token), in declaration order (ENTRY_VERSION
   bumped: an old entry is a clean miss).
2. **A declaration parse.** parser.parse_decl(name, src, range) lexes only
   the range (lexer.lex_from, as parse_body does for a lazy body) and parses
   one top-level declaration, spans in file offsets, lazily (bodies
   skipped) as the file's parse would have.
3. **Per-declaration trees.** A record file's tree starts as its declaration
   list with every entry pending; ensure_decl(f, i) parses entry i once.
   need_tree(o) parses o's declaration and its receiver type's, not their
   modules; dump.xo's generic loop parses each generic declaration, not its
   module. ensure_tree(m) stays for the readers that walk a whole file
   (the 13 that iterate ast.decls: embed, cli, comparable, timelayout,
   testmode, build.xo's 2442, decl.xo's, stmt.xo's), each audited: one that
   only runs for program modules stays as it is; one that can reach a
   record file calls ensure_tree first.
4. **Off switch and proof.** XO_STD_DECL_PARSE=off parses whole modules as
   today. A parser test parses every declaration of every std file both ways
   and compares the trees (as the lazy parse's test does); the goldens on
   every backend, the fixed point, scripts/identity on the sample, and
   TestStdChecks.

## Consequences

- An HTTP edit parses about 38 KB of std's native sources instead of
  335 KB; the bound on the saving is about 9 ms of CPU of the 14 (to be
  measured, xo bench --ab, 150 rounds, the HTTP edit build).
- The record grows by two integers per declaration.
- A reader that walks a record file's declarations without ensure_tree
  would see pending entries: the audit in step 3, and a fault on a pending
  entry instead of a silent skip.

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
