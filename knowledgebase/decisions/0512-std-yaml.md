# 0512: std/yaml, a port of gopkg.in/yaml.v3

Status: Accepted (owner, 2026-11-05), with the answers to the three open questions below ("go with your recommendations").

Landed 2026-11-05 (std/changes/yaml.md has each entry): stage 1 typed decode and encode (c1b604865), stage 2 inline, `-`, strict, decode_all, encode_all, stage 3 Duration, Bytes, parse, parse_all, encode_node, to_json, stage 4 comments in Node, stage 5 `yaml.Node` fields. Not done: Time fields (Xo's Time has no sub-millisecond precision or zone, so it cannot match yaml.v3 value for value; it needs a Time or a layout decision first), inline Maps. One deviation of the public API from the sketch below: `std/yamlcodec` was merged into `std/yaml` (an import cycle otherwise).

## Context

Backends read YAML for configuration and Kubernetes manifests. std has
`std/json`, `std/toml` (decision 0164), `std/xml` and `std/xmlcodec`,
but no YAML (checked on main 2026-11-05: no `std/yaml`, no spec section;
the only mention is a list of Go packages in decision 0412). Go's
ecosystem reads YAML through `gopkg.in/yaml.v3` (v3.0.1: MIT for the Go
port, Apache-2.0 for the C libyaml code it was translated from; its
NOTICE file lists both), the library `sigs.k8s.io/yaml` and most tools
build on.

Measured on 2026-11-05 (Go 1.26, `gopkg.in/yaml.v3` v3.0.1, a 920 byte
Kubernetes Deployment, Apple M4 Pro; the Go benchmarks become
bench/perf/yaml's twin with stage 1):

| yaml.v3 operation | time | allocations |
| --- | --- | --- |
| `Unmarshal` into a struct | 32.3 us (28.5 MB/s) | 478 allocs, 32.2 KB |
| `Unmarshal` into `any` | 38.6 us (23.9 MB/s) | 630 allocs, 40.0 KB |
| `Marshal` of that `any` | 29.2 us | 293 allocs, 84.6 KB |
| `Decoder` over 200 such documents (184 KB) | 6.07 ms (30.5 MB/s) | 90,045 allocs, 4.39 MB |

yaml.v3 is slow (an allocating event and token pipeline, a reflect based
decoder), so the twin row has room to win, as `std/json` and `std/toml`
did.

Conformance of yaml.v3 itself against the official yaml-test-suite (the
351 case copy with `in.json` and `error` files, MIT, in goccy/go-yaml's
testdata; to be vendored with attribution):

- 231 cases carry an expected JSON value: yaml.v3 passes 193 (83.5%),
  rejects 33 valid documents (for example `allowed-characters-in-alias`,
  `anchors-with-colon-in-name`, `escaped-slash-in-double-quotes`, the
  `%YAML` and `%TAG` directive cases, tab handling in flow collections,
  `zero-indented-block-scalar`) and differs on 5
  (`anchor-with-colon-in-the-middle`, `construct-binary`,
  `question-mark-at-start-of-flow-key`, `spec-example-2-27-invoice`,
  `spec-example-6-28-non-specific-tags`).
- 78 cases are invalid YAML: yaml.v3 rejects 66 and accepts 12 (for
  example `dash-in-flow-sequence`, `invalid-comma-in-tag`,
  `wrong-indented-flow-sequence`).

Xo prototype (2026-11-05, branch `yamlproto`, not pushed): a block YAML
parser of about 200 lines of Xo that works on views of the text (block
mappings and sequences, plain and quoted scalars, comments; no flow
collections, escapes, anchors, tags or documents) parses the 920 byte
Deployment into a node tree 16 times faster than yaml.v3 parses it into a
`yaml.Node`: 36.3 ms against 588.8 ms for 20,000 parses (ratio 0.062,
interval 0.057 to 0.065, `xo bench --ab`, 7 rounds, linux/arm64 host 2;
both sides print the same checksum, 11,080,000). It is a subset, so it is
a floor on the cost, not the full port's figure: the scanner's simple
key lookahead, the full escape and block scalar rules, the resolver and
`derive Yaml` still have to be paid for. It does show that a parser
without a token pipeline and per scalar allocations has a large margin
over yaml.v3 (28 to 30 MB/s), so the twin row target of beating yaml.v3
is realistic.

Xo full stage 1 port (2026-11-05, branch `yamlproto`, not pushed; reader,
scanner, parser, node composer, resolver, timestamps; about 3,100 lines of
Xo): on the yaml-test-suite it matches yaml.v3's behavior: of the 231
cases with an expected value 195 match (multi-document cases included), the
same 33 valid documents are rejected and 3 differ (all three among yaml.v3's
own 5); of the 78 invalid cases 66 are rejected and the same 12 accepted.
Twin rows (`bench/perf/yamlfull`, `xo bench --ab` style, 7 rounds, linux/arm64
host 2, the 920 byte Deployment 20,000 times, checksums equal):

- parse into a node tree against yaml.v3 into a `yaml.Node`: 395 ms against
  616 ms, ratio 0.642 (0.635 to 0.692), RSS 0.141 of Go's. This replaces
  the subset prototype's figure as the full port's cost.
- decode into a typed struct against `yaml.Unmarshal`: 451 ms against
  679 ms, ratio 0.663 (0.638 to 0.699), RSS 0.146. **This row is the interim
  route, not the design:** it goes YAML to JSON text to `derive Json`, the
  route this ADR rejects (`name: 1` into a `Str` and `yes` into a `Bool` need
  the type-directed rules of `derive Yaml`). It shows the parser's cost with
  a cheap decode; the row for the type-directed `derive Yaml` decoder replaces
  it when that exists, and the owner should judge the design on that row.
- decode into the same struct through `derive Yaml`, the type-directed
  design (`bench/perf/yamlderive`; compiler/check/derive.xo expands it as
  `derive Xml` is, to ordinary functions reading the node tree by the field
  types): 407 ms against 697 ms, ratio 0.584 (0.576 to 0.606), RSS 0.139.
  **This is the row that stands for the design.** `name: 1` into a `Str`
  gives "1", `yes` into a `Bool` true, a float into an `Int` truncates,
  merge keys and aliases apply, a mismatch is `line 2: cannot unmarshal
  !!str `abc` into int`; one `as "name"` serves Json and Yaml, with
  `yaml "name"` overriding for this format (prototype on the branch:
  decode only, the first error, no `inline`/`omitempty`/`flow` yet).

So "a port of yaml.v3" and "YAML 1.2 conformant" are different goals.
Std follows Go's standard library and, for these, Go's de facto library
(the port rule of CLAUDE.md: port Go's code and tests, Go's behavior is
the oracle).

## Decision

1. **`std/yaml` is a port of yaml.v3 v3.0.1 into Xo**, pure Xo and
   `go_compiled` (one source, `std/native/yaml.xo`, serving every backend
   as `std/jpeg` does; no cgo, no libyaml). Attribution: the yaml.v3
   license and NOTICE text in `std/native/yaml.xo`'s header and in
   `std/changes/yaml.md`. The pipeline is yaml.v3's: reader, scanner
   (scannerc.go), parser (parserc.go), node composer, resolver
   (resolve.go), decoder (decode.go), encoder and emitter (encode.go,
   emitterc.go), sorter (sorter.go: natural key order for encoded maps).
   Estimated size 7,000 to 9,000 lines of Xo (std/toml is 1,379 lines for
   a much smaller grammar; the jpeg decoder port 1,645 lines for about
   1,500 Go lines; yaml.v3 is 10,500 Go lines without tests).
2. **API** (module `std/yaml`):

   ```xo
   enum YamlErr { Syntax(line: Int, msg: Str), Type(errors: List[Str]), Encode(msg: Str) }
   fn (e: YamlErr) display() -> Str

   struct Node { kind: NodeKind, style: Style, tag: Str, value: Str, anchor: Str,
                 alias: Node?, content: List[Node], head_comment: Str, line_comment: Str,
                 foot_comment: Str, line: Int, column: Int }

   fn decode[T: Yaml](text: Str, strict: Bool = false) -> T ! YamlErr
   fn decode_all[T: Yaml](text: Str, strict: Bool = false) -> List[T] ! YamlErr
   fn parse(text: Str) -> Node ! YamlErr          // the first document, as yaml.v3's Node
   fn parse_all(text: Str) -> List[Node] ! YamlErr
   fn to_json(text: Str) -> Str ! YamlErr         // like toml.to_json, first document
   fn encode[T: Yaml](v: T, indent: Int = 4) -> Str ! YamlErr
   fn encode_all[T: Yaml](docs: List[T], indent: Int = 4) -> Str ! YamlErr
   fn encode_node(n: Node, indent: Int = 4) -> Str ! YamlErr
   ```

   `strict` is yaml.v3's `KnownFields(true)` (an unknown key is an
   error). The encoder indents by 4 by default, as yaml.v3 does (not the
   2 that most tools write), so Go's output can be matched byte for byte;
   `indent` changes it. `Node` is the dynamic form (as `json.Value`
   serves JSON) and carries comments, anchors and positions, as in
   yaml.v3.
3. **`derive Yaml`**, on the rule of decision 0315: `as "name"` stays the
   shared wire name and a field may override it for YAML with
   `yaml "name,omitempty"`, `yaml "name,flow"` or `yaml ",inline"`
   (yaml.v3's tag options: `omitempty`, `flow`, `inline`, and `-` to skip).
   Decoding is **type directed**, which is why YAML does not take the
   TOML route of converting to JSON text and reusing `derive Json`: a
   plain scalar's meaning depends on the target field (`1` into a `Str`
   field is the text `1`, `yes`, `on`, `y` and `no` are booleans only
   into a `Bool` field, `0o14`, `0x1f` and `1_000` into integers,
   `2026-11-05` into `Time`, `1m30s` into `Duration`, `!!binary` into
   `Bytes`), which JSON text cannot carry. Supported field types: Str,
   Int, Float, Bool, Option, List, Map (non-string scalar keys read as
   their text, as yaml.v3 does into `map[string]T`), nested Yaml
   structs, Time, Duration, Bytes, and `Node`. Merge keys (`<<`), anchors
   and aliases are resolved as in yaml.v3, with its alias expansion
   limit (`document contains excessive aliasing`) and its nesting limit.
4. **Errors keep yaml.v3's texts** (`yaml: line 3: did not find expected
   key`, `yaml: unmarshal errors:\n  line 2: cannot unmarshal !!str `x`
   into int`), so the differential compares them.
5. **Fidelity policy: port yaml.v3's behavior, including its
   deviations**, so the differential against yaml.v3 is exact; the
   yaml-test-suite is reported as a conformance matrix (cases passed,
   with yaml.v3's 38 and 12 known deviations listed in a committed file,
   `testdata/corpus/yaml/yamlv3-deviations.txt`) and any of them fixed
   later is its own decision. This is the owner's question below.
6. **Tests**: (a) `scripts/stdcorpus/yaml.go` with an oracle (a separate
   Go module, as the bcrypt oracle) generating a corpus of documents
   (the Kubernetes and config shapes, every scalar form of the 1.1/1.2
   core schema, every collection and block scalar style, anchors, merge
   keys, multi document streams, errors) with yaml.v3's result as
   `to_json` text or error text, and typed decode and encode results;
   (b) the yaml-test-suite vendored under `testdata/` with its license:
   `in.json` equality and the error cases, per backend; (c) `TestYamlFuzz`
   in internal/stdtest: a segment aware mutator over the corpus and the
   suite, fresh seeds, byte for byte against yaml.v3 (accept or reject,
   result or text), arm64 and LLVM; (d) encode: byte equality with
   yaml.v3's `Marshal` on generated values and `decode(encode(x)) == x`.
7. **Twin row** `bench/perf/yaml` against yaml.v3 (the Go twin as
   `twin.go.txt` with a vendored module, as bench/perf rows with
   external modules do): decode into a struct, decode to `Node`, encode,
   and a 200 document stream; baseline above. Acceptance: reported
   honestly on linux/arm64 with `--ab`; a row slower than yaml.v3 is a
   gap to close before the decision's status becomes Implemented.
8. **Stages**: (1) reader, scanner, parser, node composer, `parse`,
   `to_json`, the suite's events and JSON; (2) resolver and typed
   decode with `derive Yaml`; (3) emitter and `encode`; (4) comments in
   `Node`, merge keys and the limits, fuzz and twin rows. Each stage
   lands with its corpus and a `std/changes/yaml.md` entry; the spec gets
   section 51 with stage 1.

## Consequences

- Xo gets YAML for configuration and Kubernetes with Go's behavior as the
  oracle, a new std module of about 8,000 lines to maintain, and a
  `derive Yaml` the compiler must support (a derive of the shape of
  `derive Xml`, decision 0315).
- Porting yaml.v3's deviations means Xo rejects 33 valid YAML 1.2
  documents of the official suite (anchors with `:` or unicode, `\/` in
  double quotes, `%YAML` 1.3 style directives, some tab placements) and
  accepts 12 invalid ones, until a later decision fixes each. Programs
  that move between Go and Xo see the same behavior, which is the point
  of a port.
- The Go backend runs the same Xo source (no hand mirror), so a parser
  bug shows on every backend at once.
- The measured baseline (28 to 30 MB/s, about 480 allocations for a 920
  byte document) is a low bar: a scanner that works on the byte view
  without per token allocation should beat it; the first measured figure
  from the full stage 1 port replaces the subset prototype figure above
  when stage 1 lands.

## Owner's answers (2026-11-05)

1. **Port yaml.v3's behavior as is, deviations included.** The reference is
   yaml.v3 v3.0.1, not the yaml-test-suite: where yaml.v3 differs from the
   suite or from the YAML spec, Xo follows yaml.v3. Known deviations are
   documented in the module's doc comment and in spec/std.md: it rejects
   some valid suite documents (33 of the 231 valid ones in the harness), a
   float scalar decodes into an Int by truncating (1.5 becomes 1; checked
   against yaml.v3 itself), `yes`/`on` read as booleans only into a Bool,
   a decode reports every type error together ("unmarshal errors:"), and
   only the first document of a stream is decoded.
2. **Indent 4 by default** (as yaml.v3's Marshal), **with an option for 2**
   (yaml.v3's `SetIndent`): the encode functions take the indent.
3. **Comments in `Node` later (stage 4)**, not in the first stages;
   `parse` without comments serves config reading.

Staging: (1) parser, Node and the resolver; (2) typed decode through
`derive Yaml`; (3) the emitter, `encode` and `derive Yaml` encoding with
`omitempty`, `flow` and `inline`; (4) comments. Each stage lands with its
std corpus (scripts/stdcorpus yaml against yaml.v3), a fuzz harness, a
twin row (bench/perf), the derive goldens, TestDerivableTable and the spec.

## Open questions for the owner (answered above)

1. Port yaml.v3's behavior including its suite failures (recommended for
   v1: exact differential), or target the yaml-test-suite and diverge
   from yaml.v3 where it is wrong (fewer Go parity guarantees, a larger
   corpus of our own expectations)?
2. Default indent 4 as yaml.v3 (recommended for byte equality with Go) or
   2?
3. Comments in `Node` in stage 1, or from stage 4 (recommended: stage 4;
   `parse` without comments serves config reading)?

## Alternatives

- **Convert to JSON text and reuse `derive Json`** (the std/toml route):
  smallest, but scalar meaning depends on the target type in YAML, so
  `name: 1` into a `Str` field and `enabled: yes` into a `Bool` field
  fail or differ from Go. Kept only for `to_json`.
- **A port of goccy/go-yaml**: passes more of the suite (it is the
  source of the suite copy used above) and has a richer AST, but its
  results differ from yaml.v3's in places (different error texts, a
  different encoder), so there is no single Go oracle for byte equality
  and Go programs already written against yaml.v3 behave differently.
- **`use go "gopkg.in/yaml.v3"`** (bench/usego-workflows/yaml shows it
  works): available now, but the Go backend only, with Go's reflection
  based API and no native or LLVM build; kept as the interim for programs
  that need YAML before stage 2 lands.
- **Bind libyaml**: a C dependency, against the zero-C goal.
- **A new YAML 1.2 parser written from the spec**: no Go oracle for the
  differential and the suite alone decides correctness; larger risk for
  the same API.

## Prototype figures, type directed `derive Yaml` (2026-11-05, branch only)

Last verified: 2026-11-05. linux/arm64 offload host, `xo bench --ab` style twin row
`bench/perf/yamlderive` (a Kubernetes style Deployment, 20000 iterations, n=6, Xo
over the Go twin, lower is better):

| workload | Go ms | Xo ms | Xo/Go | range | rss |
|---|---|---|---|---|---|
| decode (`yaml.decode[T]`, collecting readers) | 719.2 | 436.5 | 0.607 | 0.576-0.619 | 0.15 |
| encode (`yaml.encode_value[T]`) | 274.9 | 170.6 | 0.62 | 0.587-0.629 | 0.155 |

Columns as the twin row prints them (Go, Xo, ratio); see
`bench/perf/yamlderive/twin.txt`.

Correctness: 442 typed decode inputs (91 hand written cases plus the yaml-test-suite
documents) give yaml.v3 v3.0.1's value or full error text; encode output is
byte-identical to `yaml.Marshal` on a 14 field struct
(`bench/perf/yamlderive/{diff,enc}`). Not covered yet: `inline`, `omitempty`, `flow`,
comments, the Go backend.
