# net/http's observable quirks the native server must copy

Status: Observed (2026-10-08, decision 0074; TestHTTPParseDiff found each)

The native HTTP/1.1 server reproduces net/http byte for byte, and the
differential test against a Go server found these, none of which is
obvious from the RFCs:

- **Two token matchers.** `shouldClose` (transfer.go) uses
  httpguts.HeaderValuesContainsToken (comma separated, trimmed), but
  `wantsClose`, `wantsHttp10KeepAlive`, `expectsContinue`, and the
  Connection checks of `writeHeader` use server.go's `hasToken` (any
  substring between space, comma, or tab boundaries). `Connection: . keep-alive`
  keeps an HTTP/1.0 connection open by the second and not the first.
- **Peek(4) between requests.** After a response, `conn.serve` peeks 4
  bytes; fewer before the end of the stream close the connection without
  an answer, while 4 or more garbage bytes get a 400.
- **An invalid handler Content-Length** is deleted from the handler's
  header, but `writeHeader` uses a copy made before the deletion: no
  automatic Content-Length, so the body is chunked (HTTP/1.1) and the
  invalid value is dropped with the chunking, or written as is (HTTP/1.0).
- **The 100-continue reader** sees the end of a Content-Length body on
  its last read (body.readLocked returns io.EOF with the last bytes), so
  it closes the connection only when the body came short.
- **A short Content-Length body** (the client stopped) does not close the
  connection; a chunked body that fails does (the chunked reader's error is
  sticky).
- **`OPTIONS *`** never reaches the handler: net/http answers 200 with
  `Content-Length: 0` (globalOptionsHandler).
- **HTTP/0.0** is HTTP/1.1 to readTransfer (Transfer-Encoding checks run,
  501 comes before 505).
- **Trailers** end at the first empty line, a bare LF too, but only once a
  double CRLF is visible within 4096 bytes (seeUpcomingDoubleCRLF).
- **101** keeps the handler's Connection header only for a protocol
  switch (Upgrade set and Connection containing `upgrade`).

How to avoid surprises: change xo_http.c only together with a corpus
entry in `internal/llvm/http_diff_test.go`, and run it with many seeds
(`XO_HTTP_DIFF_SEED=n XO_HTTP_DIFF_N=30000`).
