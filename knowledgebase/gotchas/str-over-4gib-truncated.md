# A Str or Bytes built past 4 GiB came out cut, with no fault

Status: Observed (2026-10-22)

Symptom: xo's location list text for a 24,000 line function (more than
4 GiB of assembly, gotchas/dwarf-stub-locations-quadratic.md) lost its
end, and the assembler said ".Lxo_dwllend_... is used but not defined".

Cause: a Str or Bytes length is 32 bits of its second word (decision
0330). rt.rs_set_len keeps the low 32 bits of the length it is given,
and std/native/llrt raw.xo str_join (List.join) checked no bound: it
allocated and copied the whole 4.3 GB and set a length mod 2^32.
raw.bytes_buf, str_at, fill and scan.to_hex had the same gap;
str_repeat and the append paths (cabi.xo len_over) already faulted.

Fix: raw.len_ok faults ("Str or Bytes longer than 4 GiB (4294967295
bytes)") and those allocators call it before allocating; str_join
faults with "strings: Join output over 4 GiB". internal/arm64
TestStrJoinPast4GiB joins two strings of 2 GiB + 1 MiB. A new runtime
path that sets a length it computed must check it the same way.
