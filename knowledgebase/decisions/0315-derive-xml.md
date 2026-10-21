# 0315: derive Xml, std/xml stage 2 (Go's Marshal and Unmarshal)

Status: Proposed (drafted by Xo Lang Lib 1 on 2026-10-21, for the
owner's decision on the field syntax)

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
2. **The field syntax: a second wire string, after `xml`.** Recommended:

   ```
   struct Item {
     id as "id" xml "ID,attr": Str
     title: Str                              // <title> in XML, "title" in JSON
     body xml ",chardata": Str
     tags xml "tags>tag": List[Str]
     note as "note,omitempty" xml "note,omitempty": Str? = None
   } derive Json, Xml
   ```

   - `xml` is a contextual word, only after a field name or its `as`
     string, so it is not a new keyword.
   - The string takes Go's tag text unchanged, so a Go struct ports by
     copying its tags.
   - Without an `xml` string, the field's XML name is its `as` key (its
     JSON key) when there is one, else its name. This matches Go, which
     uses the field name when a struct has no xml tag.
   - Errors get new XO codes:
     - an unknown option;
     - two fields with one XML name in one role;
     - `chardata` or `innerxml` on a field that is not Str;
     - a path (`a>b`) combined with `attr`.
3. **Stages.**
   - First encode_value and decode with attributes, chardata, paths,
     omitempty, and `-`.
   - Then innerxml and comment.
   - Each stage is tested against a Go-generated corpus of structs
     marshaled by Go, through scripts/stdcorpus with generated Go types.

## Consequences

- Programs read and write SOAP, RSS, and S3 bodies as typed structs.
- Compiler work goes where Json's does:
  - the checker parses the second wire string (decl.xo wire_keys);
  - compiler/ssa and compiler/gen generate the encoder and decoder, as
    they do for Json;
  - the Go backend calls encoding/xml on generated Go struct tags, which
    is the simplest path there.
- The wire string rules of 0277 and 0309 stay as they are for Json and
  SqlRow.

## Alternatives

- **One wire string for every derive, with XML-only options (`attr`,
  `chardata`) that Json ignores.** No new syntax, but a field whose JSON
  key and XML name differ cannot be written. Go keeps the tags separate
  for that reason.
- **A general per-derive form, `as json "id" xml "ID,attr"`.** More
  uniform, but it changes how existing `as` fields read, and Json is the
  common case.
- **A Go-tag string, `` `json:"id" xml:"ID,attr"` ``.** Familiar to Go
  users. It is a string in a string, its keys are not checked as names,
  and Xo has no backquoted tags elsewhere.
- **No derive: the tree API only (stage 1).** Every program would then
  walk elements by hand, which is what stage 2 removes.
