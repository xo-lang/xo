# glibc getaddrinfo accepts a port above 65535

Status: Observed (2026-10-09, first Linux run of the native goldens)

Symptom: the `net_echo` golden printed `listened on a bad port` on
linux/arm64 with the LLVM and arm64 backends: `os.net.listen
("127.0.0.1:99999")` succeeded. On macOS it fails (`bad port: Io`), as it
does with the Go backend everywhere.

Cause: with `AI_NUMERICSERV`, glibc's getaddrinfo parses the service and
stores it in the 16 bit `sin_port` without a range check, so 99999 binds
port 34463. Darwin's getaddrinfo rejects it.

Fix: `std/llrt/xo_rt.c` checks the port itself (`port_out_of_range`) in
`xo_net_listen` and `xo_net_dial_timeout` and fails with Go's text
(`address 99999: invalid port`). Do not rely on libc to validate input
that Go validates; check it in the runtime.
