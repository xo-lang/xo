# XO0520 on a `var` copied from a List[Int] element

Observed 2026-10-15 (std/native/llrt/sha1.xo).

Symptom: `var e = st[4]` followed by `e = d` in a loop fails with
`error XO0520: mutation of 'e' is never written back ... 'e' is a copy
of a collection element taken here`, although `e` is an Int that was
never meant to change the list.

Cause: the lost write check (core 5.2) treats any `var` initialized
from a collection element and then assigned as a possible lost update,
whatever the element type.

Fix: initialize from an expression rather than the bare element
(`var e = st[4] & M32`, or `st[4] + 0`), or name the binding for what it
is and write the list back when that is the intent.
