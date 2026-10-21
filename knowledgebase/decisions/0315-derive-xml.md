# 0315: derive Xml, std/xml stage 2 (Go's Marshal and Unmarshal)

Status: Accepted (owner, 2026-10-21), as the general rule below; drafted
by Xo Lang Lib 1 the same day. The owner revised the first draft, which
gave only XML its own string, into one rule for every format.

## Context

Decision 0314 gave std/xml Go's tokens, an element tree, and the
Encoder. Go's other half is `xml.Marshal` and `xml.Unmarshal` over
structs, driven by `xml:"..."` tags. A tag names the element or
attribute and says how the field maps:

- `name,attr` for an attribute;
- `,chardata` for the element's text;
- `,innerxml` for its raw contents;
- `,comment` for a comment;
- `a>b>c` for a path of nested elements;
- `,omitempty` to leave an empty field out;
- `-` to skip the field.

A struct's own element name comes from an `XMLName xml.Name` field, or
from its type name.

Xo derives Json and SqlRow from a struct. A field's one wire name, `kind
as "type"` (decision 0277), is shared by both, and 0309 added options
after a comma. XML names and roles differ from JSON keys often enough
(`ID,attr` beside a JSON `id`) that one string cannot serve both. That
is the question for the owner.

## Decision (proposed)

1. **`derive Xml`** gives a struct `xml.encode_value[T: Xml](v: T, indent:
   Str = "") -> Str ! XmlErr` and `xml.decode[T: Xml](text: Str) -> T !
   XmlErr`, with Go's Marshal and Unmarshal semantics and texts.
   - Fields of Str, numbers, Bool, Option (absent when None), List
     (repeated elements), nested Xml structs, and Time (RFC 3339) are
     supported.
   - The root element's name is the struct's name, or its `xml_name:
     xml.Name` field as in Go.
2. **The field syntax: a shared wire name, and a per-format override.**

   ```
   struct Item {
     id as "id" xml "ID,attr": Str
     created as "created_at" sql "created": Time
     title: Str                              // "title" in every format
     body xml ",chardata": Str
     tags xml "tags>tag": List[Str]
     note as "note,omitempty": Str? = None   // Json; Xml gets "note" and omitempty too
   } derive Json, Xml, SqlRow
   ```

   - `as "name"` (decisions 0277, 0309) stays the wire name shared by
     every format: Json, Xml, and SqlRow.
   - Each format may override it for itself with the same contextual
     form: `json "..."`, `xml "..."`, or `sql "..."`. The override
     replaces the name and its options for that format only.
   - Precedence, per format: the format's own string, else the `as`
     string, else the field's name (under the derive's `case:` rule,
     0277).
   - `json`, `xml`, and `sql` are contextual words, only after a field's
     name or another wire string, so they are not keywords.
   - Each string is checked against its own format's options, with XO
     codes for:
     - an unknown option;
     - two fields with one name in one role;
     - `chardata` or `innerxml` on a field that is not Str;
     - a path (`a>b`) together with `attr`.
   - An override for a format the struct does not derive is an error.
   - The `xml` string holds Go's tag text unchanged (`"ID,attr"`,
     `",chardata"`, `"a>b>c"`, `"-"`), so a Go struct ports by copying its
     tags.
   - Two consequences:
     - JSON and SQL names can now differ (`as "created_at" sql
       "created"`), which 0277's single name could not express.
     - A later derive (Toml, Csv) adds one word to the rule.

3. **Stages.**
   - First encode_value and decode with attributes, chardata, paths,
     omitempty, and `-`.
   - Then innerxml and comment.
   - Each stage is tested against a Go-generated corpus of structs
     marshaled by Go, through scripts/stdcorpus with generated Go types.

## Consequences

- Implementation notes (2026-10-21):
  - **Phase 1: the field syntax.** The parser reads `json`, `sql`, and
    `xml` strings after a field's name or `as` string (FormatWire), and
    the formatter, AST JSON, dump, and defhash carry them. The checker
    gives each format its key: the format's string, else `as`, else the
    name (FieldInfo.json_wire, sql_wire, xml_wire).
  - **Codes.** XO0423 is a format given twice. XO0424 is a format the
    struct does not derive. XO0412 and XO0422 apply to each string.
  - **What works now.** JSON keys and SQL columns can differ: golden
    wire_formats is identical on go, arm64, and LLVM.
  - **`xml` strings** are rejected (XO0424) until derive Xml lands in
    phase 2.
  - **Options of `as`.** `omitempty` still applies to Json only, and
    SqlRow ignores it as before. A `json` string's options replace the
    `as` string's for Json.

- Implementation notes, phase 2 (2026-10-21, Addendum 1 built):
  - **Where.** check/derive.xo expands in `parse_into` (load.xo
    add_derived_file): a file `PATH#derive` joins the module with the
    user's imports copied and `use std/xml`, `use std/xmlcodec`. Each line
    has the span of its field (Checker.gen_spans), and `report` maps a
    diagnostic there to the field with a note that it is generated.
    Naming (XO0101), unused imports, and warnings skip generated files.
  - **Call rewrite.** `xml.encode_value[T]` and `xml.decode[T]` are
    recorded in call_func and rewritten at the end of the check
    (`rewrite_xml_calls`) to `xml_encode__T` and `xml_decode__T`; no
    backend (ssa, gen, wasm) knows about derive Xml. XO0427 is a type
    parameter. The call's own key and file are kept, since the check's
    current file has moved on by then.
  - **Decode order.** Attributes first, then the elements in document
    order (`xmlcodec.events`), as Go reads them, so the first value error
    is Go's; a nested struct merges repeated elements (`xml_merge__T`).
    The document is parsed whole first: a syntax error anywhere wins.
  - **Text.** Text is written as `Node.Raw` with Go's EscapeText
    (newline as `&#xA;`), which the Encoder's CharData does not do; a
    float is Go's shortest `g` (Xo's text without the `.0`).
  - **Tools.** `xo symbols`, `def`, `refs`, and the edit commands skip
    generated files (SrcFile.generated); `xo fmt` never sees them, since
    it formats the parse.
  - **std/xmlcodec** is a new std module written once (decision 0312:
    std/native/xmlcodec.xo, listed in std/go_compiled.txt, no std/rt
    file); only generated code calls it.
  - **Speed** (100,000 documents, one run each, arm64 Mac): Go's
    encoding/xml Unmarshal 1,141 ms and Marshal 357 ms; native derive Xml
    decode 1,352 ms (1.2x) and encode 314 ms (0.9x); the Go backend
    compiled from the same Xo, 1,887 ms and 736 ms. Output byte counts
    match.
  - **Not yet.** `innerxml`, `comment`, `any`, namespaces in tags,
    XMLName, Time, Map, and Bytes fields.

- Programs read and write SOAP, RSS, and S3 bodies as typed structs.
- Compiler work goes where Json's does:
  - the checker parses the second wire string (decl.xo wire_keys);
  - compiler/ssa and compiler/gen generate the encoder and decoder, as
    they do for Json;
  - the Go backend calls encoding/xml on generated Go struct tags, which
    is the simplest path there.
- The wire string rules of 0277 and 0309 stay as they are for Json and
  SqlRow.

## Addendum 1 (2026-10-21): derive expansion in the checker (owner chose B)

The owner chose, for `derive Xml`, expansion in the checker into ordinary
Xo declarations, over per-backend generators in compiler/ssa and
compiler/gen (as Json has, 0150). The compiler stays unchanged below
the checker; a backend sees plain Xo, so the Go, arm64, LLVM, and wasm
backends all get derive Xml at once. Toml and Csv reuse the mechanism.

1. **Where it runs.** After a module's files are parsed and before its
   declarations are collected (check/load.xo, where parse results become
   the module's decls), a new check/derive.xo walks each struct with
   `derive Xml` and produces source text, which parser.parse_file reads
   and whose decls are appended to the module. Nothing is read from disk,
   and the result is a function of the struct's declaration, so the
   parse cache and the std tree hash are unaffected (the expansion is not
   cached on its own; it is cheap).
2. **What it generates**, for a struct `T` with wire names from phase 1
   (FieldInfo.xml_wire):
   - `fn (v: T) xml_tree() -> xml.Element`, the encoder: one statement
     per field that adds an attribute, a text child, a child element,
     the elements of a list, or a path of nested elements (`a>b>c`),
     and leaves an empty field out under `omitempty`. `xml.encode_value`
     and `xml.encode_indent` render the tree with the Encoder of 0314,
     so the text is Go's Marshal text.
   - `fn xml_from_tree__T(e: xml.Element) -> T ! XmlErr`, a free
     function, the decoder: reads each field from the element as Go's
     Unmarshal does (attributes, chardata, paths, repeated elements into
     a List, an absent element as None for an Option, Go's errors), then
     builds `T{...}` once with every field. A field that is absent and
     not optional keeps its zero value as Go leaves it (Str "", Int 0,
     Bool false, List empty), so decode never needs a default in the
     struct.
   - `interface Xml { fn xml_tree(self) -> xml.Element }` in std/xml is
     the constraint, satisfied structurally by the generated method.
3. **How `decode[T]` reaches the free function.** std/xml declares
   `fn decode[T: Xml](text: Str) -> T ! XmlErr` bodyless, like
   json.decode, and compiler/ssa lowers it where it lowers json_dec: it
   looks up `xml_from_tree__<T>` by the instance's type (the generic's
   substitution is known there) and calls it on the parsed tree. This
   is a lookup of a generated Xo function, about 20 lines, not a
   generator. A generic caller `fn f[T: Xml]` works the same, since the
   instance's T is concrete at lowering.
4. **Spans.** Every decl the expansion adds has its spans rewritten to
   the derive's field (the struct's `derive Xml` for the whole-type
   parts), so a diagnostic from generated code points at the user's
   source, never at synthesized text. Each field's code is its own
   helper function (`xml_enc__T__i`, `xml_dec__T__i`), so a type error
   in a field's expansion is attributed to that field, and the message
   says `(generated by derive Xml for field `x`)`. The wire string's own
   mistakes (unknown option, `attr` with a path, `chardata` on a non Str)
   are reported earlier, by wire_keys at the field (XO0412 and a new
   Xml code claimed with code-claim.sh).
5. **Tools and the formatter.** The expansion happens after the parse,
   so `xo fmt` (which formats the parse of the file) never sees the
   generated decls and cannot rewrite them. Generated FnDecls carry
   `generated: true`; `xo def`, `xo refs`, `xo symbols`, and `xo doc`
   skip them as declarations (`xo doc T` may list `xml_tree` marked
   generated), and the name `xml_tree` cannot be written by the user for
   a type deriving Xml (the existing duplicate-method code applies, with
   the note that derive Xml defines it).
6. **Toml and Csv.** check/derive.xo holds the shared part: the field
   walk (wire name, role, options, type shape: scalar, Option, List,
   nested derived struct), the span rewrite, and the generated-flag.
   A format supplies a template per type shape. A later `derive Toml`
   or `derive Csv` adds a template file and its std module's tree type,
   with no change to ssa or gen beyond the one lookup of item 3.
7. **Proofs.** The Go-generated corpus (scripts/stdcorpus xml, structs
   marshaled and unmarshaled by Go) on go, arm64, and LLVM; errorcheck
   cases for bad wire strings reported at the field and for a type error
   inside generated code reported at the field; `xo fmt` and `xo def`
   unchanged on a file with a derived struct; and a speed comparison
   with Go's encoding/xml Marshal and Unmarshal on the corpus, reported
   to the lead. If the generated code is much slower than Go's, the
   remedy is in the generated Xo (and in the tree), not a return to
   backend generators.

Consequences: expansion cost is one extra parse per deriving struct (a
few small functions); the tree is built first, so encode and decode
allocate an Element tree the backend generators would avoid, which the
speed comparison measures.

## Alternatives

- **The first draft: an `xml` override only, the other formats sharing
  `as`.** The owner generalized it, since the same need (one name per
  format) exists for Json and SqlRow.
- **One wire string for every derive, with XML-only options (`attr`,
  `chardata`) that Json ignores.** No new syntax, but a field whose JSON
  key and XML name differ cannot be written. Go keeps the tags separate
  for that reason.
- **A Go-tag string, `` `json:"id" xml:"ID,attr"` ``.** Familiar to Go
  users. It is a string in a string, its keys are not checked as names,
  and Xo has no backquoted tags elsewhere.
- **Per-backend generators in ssa and gen (as Json, option A).** No
  intermediate tree and the fastest code, but two more generators for
  each format (and again for Toml and Csv), each tested on every
  backend. The owner chose expansion (Addendum 1).
- **No derive: the tree API only (stage 1).** Every program would then
  walk elements by hand, which is what stage 2 removes.
