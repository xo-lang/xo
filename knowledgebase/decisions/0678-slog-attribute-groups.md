# 0678: Attribute groups in std/log (slog.Group, Logger.WithGroup)

Status: Proposed (Xo Lang 3, 2026-11-16; task #89)

## Context

Go's log/slog nests attributes in groups: `slog.Group("req",
"method", "GET", "path", "/x")` is one attribute whose value is a list of
attributes, and `logger.WithGroup("req")` puts every attribute added
after it (by later `With` calls and by each record) under that name. The
text handler writes a group's members with dotted keys
(`req.method=GET req.path=/x`); the JSON handler nests them
(`"req":{"method":"GET","path":"/x"}`). An empty group is left out, and a
group with no name (`slog.Group("", ...)`) inlines its members.

std/log (decisions 0413 and 0617 C) has flat fields only:
`Map[Str, Debug]` at the call and on `Logger.with`, `Record.fields`
(`Map[Str, Str]`) and `Record.values` (`Map[Str, Value]`). Decision 0617
left `WithAttrs` and `WithGroup` out as handler pre-rendering. Grouping
itself is not pre-rendering: it changes what a record says, and the
stdaudit rows slog.Group and slog.Logger.WithGroup are the module's two
open feature gaps (scripts/stdaudit/map.txt).

## Decision

1. `Value` gains a variant `Group(fields: Map[Str, Value])`, in the
   order the fields were given.
2. `log.group(fields: Map[Str, Debug]) -> Group` makes a field value
   that is a group: `log.info("served", {"req": log.group({"method":
   m, "path": p})})`. `Group` is a struct implementing Debug (its Debug
   text is the dotted form), so it fits the existing
   `Map[Str, Debug]` fields with no new call shape. The field's key is
   the group's name; an empty key inlines the members, an empty group is
   left out (Go's rules).
3. `Logger.with_group(name: Str) -> Logger` (slog's WithGroup): fields
   given after it, by later `with` calls and by each record's own
   fields, go under `name`; the logger's earlier fields stay where they
   were. Nested `with_group` calls nest. An empty name is a no-op (Go's).
4. `Record.values` carries the nesting (the Group variant);
   `Record.fields`, the flat `Map[Str, Str]` view, carries each member
   under its dotted key (`req.method`), so handlers written against
   `fields` keep working and see what the text handler writes.
5. The text handler writes dotted keys and the JSON handler nested
   objects, byte for byte as Go's TextHandler and JSONHandler for the
   same attributes (a golden against the Go backend running slog).

## Consequences

- A handler that matches on `Value` exhaustively must add the `Group`
  arm: a breaking change, with a CHANGELOG line saying what to do
  (match `Group(fields)` and recurse, or read `Record.fields`).
- The OpenTelemetry log bridge (std/otel's handler, if one reads
  `values`) maps a group to a map attribute, as Go's otelslog does.
- The two stdaudit gaps close; std/log's features mark can reach Proven
  with the Handler and NewMultiHandler rows mapped to `log.Handler` and
  `log.multi`, which exist.

## Alternatives

- **Dotted keys only** (`{"req.method": m}` by convention): no API, but
  JSON output differs from Go's (flat keys, not nested objects) and a
  key holding a dot cannot be told from a group. Rejected: the audit's
  bar is Go's output.
- **A separate `with_group` only, no group values**: covers request
  scoped loggers but not one attribute holding a group at a call (Go's
  most common use, `slog.Group("req", ...)` in a log line). Rejected as
  half the feature.
- **Pre-rendering handlers (WithAttrs, WithGroup on Handler)**: still
  left out (decision 0617); groups here live in the record, so a handler
  needs no new methods.
