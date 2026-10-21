# 0314: std/xml, Go's encoding/xml tokens and an element tree

Status: Accepted (owner, 2026-10-21, within decision 0276): stage 1 (tokens
and the tree). Stage 2 (a struct derive) needs its own decision.

## Context

The coverage table lists encoding/xml as planned. Services meet XML in:

- SOAP and SAML;
- sitemaps and RSS;
- TwiML and other webhook replies;
- cloud APIs that still answer in XML (S3's list and error bodies).

Go's package has two layers:

- a token stream, Decoder.Token and Encoder.EncodeToken, with namespace
  resolution and strict well-formedness checks;
- Marshal and Unmarshal over structs with `xml:"..."` tags.

Xo has neither. The owner put encoding/xml on lib1's queue on
2026-10-21.

## Decision

1. **Stage 1: a module `std/xml` with Go's tokens and a tree built from
   them.**

   ```
   struct Name { space: Str, local: Str }           // Go's xml.Name (space: the namespace URL once resolved)
   struct Attr { name: Name, value: Str }
   enum Token {
     Start(name: Name, attrs: List[Attr])
     End(name: Name)
     Text(s: Str)                                   // CharData, entities and CDATA decoded
     Comment(s: Str)
     ProcInst(target: Str, inst: Str)
     Directive(s: Str)
   }
   struct XmlErr { line: Int, msg: Str }            // Go's SyntaxError: "XML syntax error on line N: ..."
   pub fn tokens(text: Str, strict: Bool = true) -> List[Token] ! XmlErr   // Decoder.Token until EOF
   pub fn raw_tokens(text: Str) -> List[Token] ! XmlErr                   // Decoder.RawToken: no namespaces, no matching

   struct Element { name: Name, attrs: List[Attr], children: List[Node] }
   enum Node { Elem(e: Element), Text(s: Str), Comment(s: Str), ProcInst(target: Str, inst: Str), Directive(s: Str) }
   pub fn parse(text: Str) -> Element ! XmlErr      // the root element; text around it may be only whitespace, comments, PIs, a doctype
   fn (e: Element) attr(local: Str) -> Str?
   fn (e: Element) child(local: Str) -> Element?    // the first child element of that local name
   fn (e: Element) children_named(local: Str) -> List[Element]
   fn (e: Element) text() -> Str                    // the element's character data, its own and its descendants', in order
   fn (e: Element) encode(indent: Str = "") -> Str  // Go's Encoder: EncodeToken of each token, Indent("", indent)
   pub fn escape(s: Str) -> Str                     // Go's EscapeText
   ```

   - The tokens and their errors are Go's: entities (the five predefined
     and numeric references), CDATA, comments, processing instructions,
     directives, attribute value normalization, and namespace resolution
     (`xmlns` and `xmlns:p`, with Go's results for undeclared prefixes).
   - Go's Decoder options Strict, AutoClose, and Entity: `strict: false`
     is Go's `Strict = false` together with `AutoClose = xml.HTMLAutoClose`
     and `Entity = xml.HTMLEntity`, the HTML leniency.
   - Natively, std/native/xml.xo ports Go's xml.go (the Decoder and the
     escaping). On the Go backend, std/rt calls encoding/xml.
   - Safety: no external entities, no DTD processing (Go has none), and a
     nesting limit, Go's 10,000 depth (`exceeded max depth`).
2. **Stage 2 (a later decision): `derive Xml`.** Go's struct tags map
   fields to attributes, character data, inner XML, comments, and
   `a>b>c` paths. Xo's single wire name (decisions 0277, 0309) is used by
   Json and SqlRow, so XML needs a second one. The syntax is for that
   decision, with the owner.
3. **Tests.**
   - Go's xml_test.go and read_test.go token tables as std module tests.
   - A Go-generated corpus (scripts/stdcorpus xml) of random documents:
     well formed, cut short, with a byte changed, with namespaces,
     entities, and CDATA. Each is checked against Go's tokens or Go's
     error, its tree, and its encoding.
   - A golden on every backend.

## Consequences

- Programs can read and write XML documents of any shape now, with Go's
  results on every backend. Typed decoding into structs waits for stage 2.
  The tree and `child` and `text` cover the usual "read three fields"
  case.
- The whole document is in memory, as with JSON's `parse_value`. A
  streaming decoder can follow json.Decoder's push design (decision
  0309) if a user needs one.

## Alternatives

- **Structs first (derive Xml) without a token API:** the derive needs
  compiler work and a second wire name, and a tree serves the common
  case sooner.
- **An XPath subset:** beyond Go's scope (decision 0276).
- **A token stream without the tree:** Go users read tokens in a loop.
  Xo programs would then rebuild the same tree every time.
