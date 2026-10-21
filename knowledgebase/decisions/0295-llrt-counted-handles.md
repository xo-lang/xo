# 0295: One helper for counted runtime handles in llrt

Status: Accepted (lead, from the agents' poll with the owner, 2026-10-20);
reviewed by Xo Lang 3 (tlsio and netconn are theirs), 2026-10-20.

## Context

A runtime handle (a file, a lock, a connection, a TLS stream, a child
process) is a record behind the runtime's 16 byte object header: the
count, then the address of a free function the last release calls
(obj.xo h_free_n). Making one took four steps that only convention
recorded:

1. `tasks.new_handle(n, rt.fn_addr("NAME"))` (netconn.xo had its own
   copy);
2. `rt.obj_at[T](p)` for the Xo value, then `tasks.h_release(p)`,
   because obj_at counts once more;
3. a free function that releases what the fields own and then calls
   `obj.free_obj(p)`;
4. a C ABI export named NAME in a cabi*.xo module forwarding to it,
   since rt.fn_addr takes only an export's literal name.

Missing step 2's release or step 3's free_obj leaks the record silently.
Xo Lang 3 leaked an allocation that way on 2026-10-20.

## Decision

std/native/llrt tasks.xo gains:

- `make_handle[T](n, free) -> (T, Int)`: steps 1 and 2 in one call. It
  returns the value that holds the one reference and the record's address
  for writing the fields.
- `free_handle(p)`: the last call of every free hook (step 3).
- `HANDLE_FIELDS` (16): the first field's offset.
- A comment block next to them stating the rules. Rule 5 (from Xo Lang 3's
  review): a handle returned as an address through a cabi export (netconn's
  constructors) keeps new_handle, because the export's caller adopts the
  reference; its hook still ends with free_handle.

fsops.xo's file and lock handles are migrated to show the pattern.
Golden `handle_drop_leak` (every exit 0 golden runs with XO_LEAKCHECK=1)
opens and drops files and locks, closed and not, in a loop and in a
task, so a hook that stops freeing its record fails it.

## Consequences

- New handles take two calls instead of four steps. The cabi export stays
  (rt.fn_addr's rule).
- The other handles (chan, mutex, netconn, tlsio, proc, hclient, hconn,
  hserve, selects, anyval) move over as their owners touch them. They are
  correct today, so there is no flag day. Xo Lang 3 moves tlsio's
  handle_new, and netconn's private new_handle (byte for byte
  tasks.new_object) and free calls, the next time they touch those files.

## Alternatives

- One dispatching export for every handle (a hook id in the record): it
  would remove step 4, but adds a registry every new handle must edit,
  which trades one convention for another.
- Passing the hook as a function value: llrt passes only Int words to
  stay free of allocation (decision 0207's rule for rc.xo, kept by
  convention elsewhere).
- Starting new objects at zero references so obj_at's count is the one:
  the count encoding has no shared zero, and other new_handle users rely
  on one.
