# 0493: A valued enum may keep unknown values in one catch-all variant

Status: Accepted (owner, 2026-11-04). Proposed by xo-lang-4 the same day,
for the AWS SDK agent through the lead.

## Context

Valued enums (decision 0269, core 2.3) are closed: `E.from(v)` is `None`
and `derive Json` fails for a value no variant has. That is right for a
set the program owns, and wrong for a set a server owns and grows. AWS
adds enum values without notice: S3's `StorageClass` gained
`EXPRESS_ONEZONE` (2023) after SDKs that did not list it were
released, so a ListObjectsV2 response from today's S3 makes an
older client built on a closed enum fail to decode the whole page
because of one object's storage class.

The AWS SDK (repo aws, docs/decisions/0004-api-shape.md and 0006 item 2)
works around it by generating, for each enum in the models, a
plain enum with an `Unknown(value: Str)` variant and hand-written
`value()`, `parse()` and `values()` methods, plus a custom Json codec. It
asked for the language form. Decision 0269 section 3 covers open sets of
integers with a newtype and constants, but has no named variants, no
exhaustive `match`, and no `Str` form.

## Decision

A valued enum may end with one catch-all variant: a variant with exactly
one field, of the enum's value type, and no value.

```
pub enum StorageClass: Str {
  Standard = "STANDARD"
  ReducedRedundancy = "REDUCED_REDUNDANCY"
  Glacier = "GLACIER"
  StandardIa = "STANDARD_IA"
  IntelligentTiering = "INTELLIGENT_TIERING"
  DeepArchive = "DEEP_ARCHIVE"
  Unknown(Str)
} derive Json
```

- `E.from(v)` returns `E`, not `E?`: the variant with value v, else
  `Unknown(v)`. It cannot fail, so a catch-all enum has no `None` case to
  handle. `E.known(v) -> E?` keeps 0269's strict lookup (`None` for a
  value no listed variant has) for code that wants to refuse.
- `e.value()` is the variant's value, and for `Unknown(v)` it is v.
- `E.all()` lists only the named variants; `Unknown` has no single value
  to list.
- `derive Json` writes `Unknown(v)` as v and reads a value no variant has
  as `Unknown(v)`: decoding a valued enum with a catch-all never fails on
  an unlisted value (a value of the wrong JSON type still does).
- `match` stays exhaustive: a match on a catch-all enum needs an
  `Unknown(v)` arm (or `_`), which is the point: the code says what it
  does with a value newer than itself.
- One spelling per value. `Unknown("STANDARD")` would be a second
  spelling of `Standard` and compare unequal to it (equality is by
  variant, then by the field). `from` and Json decoding never make one;
  constructing the catch-all with a constant that a named variant has is
  a compile error (a new code); with a value only known at run time,
  `E.from(v)` is the way to build one, and `xo vet` flags a direct
  `E.Unknown(x)` with a non-constant x (a new warning).
- Equality, hashing and `Ord`: by variant in declaration order, so
  `Unknown` sorts last, then by its value. Display prints the variant
  name for a named variant and `Unknown(v)` for the catch-all (the
  ordinary display of a variant with a field).
- `Int` valued enums take the same form (`Unknown(Int)`), for Smithy's
  `intEnum`.
- Errors: a second catch-all, a catch-all that is not last, or one whose
  field is not the value type, each a new code beside `XO0428` and
  `XO0429` (whose meaning does not change: a variant with fields is still
  `XO0429` unless it is the one catch-all).

Worked example (an S3 client built before `EXPRESS_ONEZONE` existed,
reading today's ListObjectsV2):

```
var pages = s3.ListObjectsV2Paginator.new(client, req)
for page? in pages {                       // decision 0494
  for obj in page.contents {
    match obj.storage_class ?? StorageClass.Standard {
      Standard | IntelligentTiering => hot += obj.size ?? 0
      Glacier | DeepArchive => cold += obj.size ?? 0
      ReducedRedundancy | StandardIa => warm += obj.size ?? 0
      Unknown(v) => log.warn("storage class ${v} not known to this build")
    }
  }
}
```

The object stored as `EXPRESS_ONEZONE` decodes to `Unknown("EXPRESS_ONEZONE")`,
the page decodes, the loop goes on, and writing the object back
(`CopyObject` with `storage_class: obj.storage_class`) sends
`"EXPRESS_ONEZONE"` unchanged.

## Consequences

- The AWS SDK generates one enum declaration per Smithy enum, with no
  `value`, `parse`, `values` or Json code: its generator and its output
  shrink by those four methods for every enum in the service models.
- `from` has a different result type on a catch-all enum (`E`) than on a
  closed one (`E?`). A generic function over valued enums cannot treat
  both alike; there is no such constraint today.
- Spec: core 2.3 (valued enums) and 2.2's Json rules; spec/std.md's enum
  methods; the new codes in the 10 table and `xo explain`.
- Every backend lowers it as today's enum with a field; `from` adds the
  fallback arm. No runtime change.

## Alternatives

- Keep the generated workaround (the SDK's 0004): works, but every SDK
  that talks to a server it does not own writes the same four methods
  per enum, and Json needs a hand codec.
- A newtype over `Str` with constants (0269 section 3's open sets,
  extended to `Str`): keeps every value, but `match` has no
  exhaustiveness over the named values and `all()` does not exist.
- An attribute instead of a shaped variant (Rust's `#[non_exhaustive]`,
  Swift's `@unknown default`): marks the set open for matches, but gives
  the program no way to keep and send back the unknown value, which the
  SDK needs to round trip.
- `from` keeping `E?` on a catch-all enum: uniform, but its `None` would
  never happen and every caller would write `?? unreachable`.
- Protobuf's open enums (unknown numbers kept in the field as plain
  integers): the model taken here, with a named variant so the type
  stays a sum type.
