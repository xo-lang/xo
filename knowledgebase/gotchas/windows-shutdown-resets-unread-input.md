# A Windows socket shut down for receiving with unread input resets

Status: Observed (2026-10-12), fixed in std/llrt/xo_rt.c (pd_close).

Symptom: the http_tls golden on windows/amd64: a plain HTTP request to
the TLS server should get net/http's `400 Client sent an HTTP request to
an HTTPS server.`; the client saw `Protocol(... EOF)` instead. Every
other HTTP golden passed.

Cause: the server reads the first 5 bytes of the request, writes the
400, and closes; pd_close calls `shutdown(fd, SHUT_RDWR)` first. On
Windows, `shutdown` with SD_RECEIVE while received data is unread resets
the connection, and the reset discards what the peer had not read yet,
the 400 included. Linux and macOS send the response before the close's
reset reaches the peer.

Fix: on Windows, pd_close drops the unread input (nonblocking `recv`
until it would block) before `shutdown`. Data arriving after that can
still reset the connection; a lingering close (shut down for sending,
then read until the peer's FIN or a timeout) is the complete fix if a
golden ever needs it.
