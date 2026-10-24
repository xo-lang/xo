# TLS server read lost the head of a record cut by the handshake's last read

Status: Observed and fixed 2026-10-23.

Symptom: `http_stream_body_h2` hung in about 1 run in 300 (more under load);
an h2 streamed request body stopped after 65535 bytes, the server idle in
`wait`, kernel queues empty, `xo dump` showing the handler in body_next.
The same fingerprint as a 52 minute xo build hang.

Cause: tlssrv.fill ("the handshake takes everything") fed the whole read to
the TLS server and cleared `pend`. When that read also held the first
application records and ended inside one, `feed` finished the handshake and
kept the partial record in its own `rin`; `step` only looks at `pend`, so
the next read was appended to a record's tail. The first header read was
garbage (`ty=42`, `n=38576` in the XO_H2DEBUG-style logs), so no record was
complete and the connection waited forever. It depends on whether the
client's Finished and its first data arrive in one read, hence the load.

Fix: after the handshake `feed`, if the connection is done, `pend` takes the
server's `rin` (take_rin). Finding it: count bytes at each layer (client
send, socket read, plaintext) and log each record header; the first record
after the handshake had a nonsense type.

Why it was a 1 in 300 flake: the garbage "record header" is bytes of random
ciphertext. `step` hands a header whose length is over 16640 to feed whole
(which rejoins the leftover head with the rest by luck), so only a length
up to 16640 that is more than the bytes read (about one in four) waits
forever. Regression test: golden tls_coalesced_finish (a TCP proxy joins
the Finished and the first data in one write; 40 new connections).
TLS 1.2 shares the fix (take_rin) but not the trigger: its client sends
data only after the server's Finished.
