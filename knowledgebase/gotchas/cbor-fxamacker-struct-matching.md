# fxamacker/cbor struct decoding rules that a hand written mapping misses

Status: Observed (2026-11-05, derive Cbor, decision 0527)

## Symptom

A first derive Cbor decoder (match each key to a field, decode, report mismatches) disagreed with fxamacker/cbor v2.9.0 on
about 1% of seeded cases, in both directions (Xo refused what Go accepted, and the reverse).

## Cause

fxamacker's `Unmarshal` into a struct:

- matches a text key exactly first, then ignoring case; the first key to reach a field wins and later ones are dropped
  unchecked (a later `"Name": [1]` after `"name": "a"` is no error);
- a `keyasint` field also matches its number as text (`"7"`);
- checks tags 0 to 3 (text, number, byte string, byte string) on every value it decodes, never on a skipped one, and strips
  tag 55799 first;
- reads tag 2 or 3 over a byte string into an int when it fits, an array of small integers into `[]byte`, an integer into a
  float, and leaves a field alone for null;
- writes a nil slice or map as null unless `NilContainerAsEmpty` is set (the oracle sets it).

## Fix

`std/native/cbor.xo` (`untag`, `bignum_int`, `to_bytes`) and `cbor_codec` in compiler/check/derive.xo implement these;
the typed corpora in testdata/corpus/cbor (oracle modes `typed`, `typed2`, `typed3` of internal/stdtest/cborref) pin them.
Weight generated cases toward well typed values (about half should decode), or the error path hides the rest.
