# 0682: derive SqlRow gains an encode half; sql.named_args (task #124)

Status: Accepted (owner, 2026-11-16; proposed by Xo Lang 3, 2026-11-16; the lead asked for it before the
code; Xo Lang 1 reviews the compiler part).

## Context

pgx's StructArgs (and StrictStructArgs) pass a struct's fields as named
arguments: `@name` in the query takes the field whose `db` tag or name is
`name`. std/sql has `sql.named(q, args: Map[Str, SqlValue])` (pgx's
NamedArgs, task #113), so the missing piece is a struct's fields as that
map.

A library function over `json.to_value` cannot do it exactly: Bytes
encode as a base64 Str and Time as RFC 3339 text, identical to Str fields
in a `json.Value`, and Xo has no reflection, so a bytea column would get
the base64 text silently. Only the compiler knows each field's type.
`derive SqlRow` (decision 0278) already restricts fields to types that
map one to one onto SqlValue and already knows their wire names.

## Decision

1. `derive SqlRow` also generates the encode half: the struct's fields as
   `List[(Str, SqlValue)]` in declaration order, each under the column
   name `decode` reads (the wire name `created as "created_at"`, or the
   name under `derive SqlRow(case: ...)`). Per field type:
   - `Int` is `Integer(n)`, `Float` is `Real(f)`, `Str` is `Text(s)`,
     `Bool` is `Flag(b)`, `Time` is `At(t)`, `Bytes` is `Blob(b)`;
   - `T?` is the above for `Some`, `Null` for `None`;
   - `List[Int]` and `List[Str]` are `Array` of `Integer` or `Text`
     items (what `ints` and `strs` read).
   No other type is allowed, as for decode (XO0404 at the derive).
2. std/sql gains
   `pub fn named_args[T: SqlRow](v: T) -> Map[Str, SqlValue]` (pgx's
   StructArgs: `sql.named(q, sql.named_args(v))`), and
   `strict` on `named` already gives StrictStructArgs' check (an
   argument the query does not use is an error).
3. The encode half has no surface of its own beyond `named_args`: the
   compiler exposes it to std/sql the way decode is exposed to
   `Row.decode` (an intrinsic or a generated function; Lang 1's choice).
   `Row.decode[T](row)` of `named_args`' values gives back `v` for every
   allowed type (a round trip test on each backend).

## Consequences

- StructArgs and StrictStructArgs (scripts/stdaudit/map.txt) become
  ports; tests against Postgres through pg-diff with Bytes, Time,
  Option and List fields.
- Programs gain insert helpers without spelling each field:
  `db.exec(sql.named("INSERT ... VALUES (@id, @name)", sql.named_args(u)))`.
- No change for existing derive SqlRow users: the encode half is new
  code only used by `named_args`.
- Lang 1 owns the compiler part (all backends, wasm included); Lang 3
  the std function and tests (#124), after it.

## Alternatives

- `sql.named_args[T: Json]` over `json.to_value`: no compiler work, but
  Bytes and Time arrive as text indistinguishable from Str (wrong bytes
  in a bytea column, silently). Rejected (the lead, 2026-11-16).
- A separate `derive SqlArgs`: the same field rules and names as
  SqlRow, twice; a struct used both ways would derive both. Rejected
  for one derive with both halves.
- Leave the rows as gaps: the insert side of `query_as` stays manual.
