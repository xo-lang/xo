# 0500: On LLVM, a `var` field or list element argument is passed at its address

Status: Accepted (owner, 2026-11-04); proposed by xo-lang-4 the same day. The LLVM follow-up to
decisions 0490 (field homes, wasm) and 0498 (list element homes,
native), which the owner asked for when accepting 0490.

## Context

The LLVM backend passes a large `var` parameter (decision 0145) by
reference, and the memory of a `var` parameter is the home of its type
(decision 0148, compiler/llvm/held.xo): calls passing a value that
memory holds pass the memory itself. Every other large `var` argument
got a stack slot (an alloca per type), was stored into it before the
call, and was loaded back after it, then stored where it came from.

tlsio.put (darwin/arm64, the fetch client, `--backend=llvm`) was 7,480
bytes, with three such copies of TLS clients (968 and 1,120 bytes):

- `tls13.send(c.cl, data)`: a field of the `var` parameter c, copied to
  an alloca and back, then written into c's memory;
- `tls12.send(c.cl12[0], data)`: a taken list element (decision 0169),
  loaded whole by the take, a placeholder stored, copied to an alloca,
  loaded back, and stored into the list;
- each fault cleanup releasing c's TLS 1.3 client loaded it whole and
  stored it to a temporary to release it there.

Native already did the first since 2026-10-21 (byref.xo field homes),
and 0498 does the second there.

## Decision

On the LLVM backend:

1. Field homes (0490's rule): a `var` argument passed by reference that
   is field `idx` of a value held in a tracked memory (and that no other
   argument of the call is stored into) is passed as a GEP to that
   field. The dataflow records the memory as holding "the value with
   that field rewritten by the call"; the setfield that stores the
   call's result back at that field emits nothing, reads of the value's
   other fields load from the memory, and the call's result for the
   argument is undef when only that setfield reads it.
2. List element homes (0498's rule): a `var` argument that is the
   element of a `list.take` whose new value only goes back by the
   `list.set` at the same index (and decision 0488's ShareIn before it)
   is passed at its slot (GEP into the list's data). The take checks
   the index and loads nothing; the set and its ShareIn emit nothing;
   the taken list may otherwise only be released (a fault's cleanup).
3. A release of a field of a value held in a tracked memory releases it
   at its GEP (the drop helpers take a pointer) instead of loading it
   and storing it to a temporary.

## Consequences

- tlsio.put 7,480 to 684 bytes; the fetch client 4,496,339 to 4,479,827
  bytes (-16,512), darwin/arm64, against the same main. Speed
  (bench/results/llvm-var-homes-2026-11-04.md, linux/arm64, `--ab`, 3
  rounds, `--backend=llvm`, bench/perf/tlsloop): 16 byte requests over
  TLS 1.3 0.969x wall (0.969 to 0.989), 0.951x CPU, 0.939x peak RSS;
  16 KiB requests 0.998x wall, CPU within noise; the binary 85,416 bytes
  smaller.
- Goldens field_homes and list_elem_homes (a fault mid-update under the
  leak check, a kept copy, a list in a field, a local list) run on LLVM
  as on native.
- The rules are the native ones, so the two backends again lower these
  calls alike; wasm has 0490's field rule only.

## Alternatives

- Leave LLVM copying: clang does not remove an alloca round trip through
  an opaque call that takes its address.
- Mark the slot `noalias` and hope clang forwards the copies: it cannot
  across the call.
