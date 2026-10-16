# Raw addresses and the life of the value they point into (2026-10-16)

The lead's audit after Xo lang 5 found dwline.xo reading through the raw
address of a Bytes that had been released at its last use. Reference
counting (selfhost/ssa rc.xo, decision 0046) places a release right
after a value's last use, by a backward liveness pass, and a raw address
(raw.bytes_addr, raw.data, rt.obj_addr) is not a use. So a raw address
into a value is valid only up to that value's last use, and that use
can be inside the arguments of the very call that uses the address:
`f(raw.data(s), s.len())` releases a string `s` the function owns after
`s.len()` and before `f` runs (checked in the arm64 code of
proc.executable: the release precedes the call of access).

## Method

Every raw.bytes_addr, raw.data, rawb.*, rt.obj_addr and digest.xo
bytes_data in std/native and selfhost on main 17c3642f: 265 uses in 53
files. A script classified each by its owner (scratch, not in the repo):
a parameter of the function (135: safe, the caller holds it across the
call), a field or element of a parameter, a constant or literal
(immortal), or a local this function owns, whose last mention was then
compared with the address's last use. The 31 left were read by hand.

## Unsafe, fixed

| Site | Owner | What happened after its release |
|---|---|---|
| tlsio.xo flush | the bytes taken to send | sock.send through the address, and poll.pd_wait between sends, where other tasks allocate (the TLS client's output) |
| tlssrv.xo flush | the same | the same, the TLS server's |
| hconn.xo peer_ip | the sockaddr buffer | getpeername wrote it, then it was read |
| proc.xo spawn_linux | the pipe buffer | the exec error read after the keep |
| proc.xo executable | the C path | access(2) read it |
| h2srv.xo fragment, schedule | the HPACK block | put_block copied it |
| h2w.xo head | the sniffed content type | field copied it |
| hclient.xo start | the connection key | conn_new copied it |
| http1w.xo error_answer | the answer | mem_copy read it |
| testworld.xo serve_next (two), reset_test | the values | setenv read them |
| tlsio.xo roots_pem | a certificate's DER | str_at copied it |

Fixes: raw.alive(b) and raw.alive_str(s) (raw.xo: a length test the
compiler keeps) after the last use of the address; the two flush loops
now take the bytes as a parameter (send_all), which the caller holds.
Test: the rt_addrs golden's new line (std/native/internal.xo rt_op
"alive"): 64 bytes filled, their address taken, 64 more blocks of their
size allocated and filled, then read through the address: 0101 with
raw.alive after the read, 0001 (the block reused) without it, on arm64
and LLVM.

## Safe (read, no change)

- Parameters, fields and elements of parameters (tlscert.xo
  apple_trust_darwin's der, tzhost.xo host_data's zip), constants and
  literals (http1w.xo DAYS and MONTHS, sha256.xo K256, nistec_g256.G256,
  tasks.xo empty_list's `[]`).
- A local kept by a later use on every path that uses the address:
  darwin.xo names and readlink, dns.xo zone_index, fsops.xo copy,
  linux.xo's system calls (keep(z, r)), sock.xo wsa_init, windows.xo
  pipe, pipe_read, pipe_write, reap, win_text, inheritable, prof.xo
  elf_symbols (live across its loop), raw.xo zero_ints (returned).
- dwline.xo load_elf and load_dsym: Xo lang 5's fix.

## The rule

A raw address into a value is valid up to the value's last use. Put
raw.alive or raw.alive_str after the last access through the address,
or pass the value as a parameter of the function that uses the address.
A use inside the same call's arguments is not enough.
