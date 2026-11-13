# std/http changes

## 2026-11-13: security: an empty CONTINUATION frame costs the HTTP/2 server only its read

- Kind: security
- Commit: this push
- Found by: Lang 2's review of the GO-2023-1571 fix (decision 0616); no Go advisory (Go's hpack Decoder.Write returns at once on an empty write)
- Test: golden http_h2_empty_continuation (50000 empty frames after 800 KB of a field's value: 2.2 s before the fix, 3 ms after, on arm64; the request still answered)
- Affected: every release up to 0.50.1 (native and LLVM backends)
- Shipped in: not yet released
- Remote trigger: any HTTP/2 client (a denial of service: CPU)

Each CONTINUATION frame asked HPACK to decode again and copied the header bytes already waiting, even when the frame brought no bytes, so empty frames cost time in proportion to what was waiting. An empty fragment that does not end the block is now read and dropped.

## 2026-11-13: security: Request.query_all counts the parameters before splitting, and is empty past 10000

- Kind: security
- Commit: this push
- Found by: Lang 2's review of the std/http security fixes (decision 0616), the query_all half of GO-2026-4341 (CVE-2025-61726)
- Go reference: net/url ParseQuery (the parameter count checked before any part is made; Request.URL.Query() is then empty)
- Test: golden http_form (query_all of 10000 parameters gives 10000 values, of 10001 none; fails before the fix with 10001)
- Affected: every release up to 0.50.1, all backends
- Shipped in: not yet released
- Remote trigger: any client of a route that calls query_all (a denial of service: memory and CPU for a long run of `&`)

query_all split the raw query on every `&` before looking at any pair, and returned values past the 10000 parameter limit where Go's Query() returns none. It now counts the separators first and answers [] past the limit, as url.parse_query does since its own fix.

## 2026-11-13: security: an HTTP/2 connection buffers at most four bodies' worth of unfinished request bodies

- Kind: security
- Commit: this push
- Found by: the std/http security review (decision 0616), an extra finding (no Go advisory: net/http hands a body to its handler as it arrives and returns the window as the handler reads)
- Test: golden http_h2_body_budget (fails before the fix: eight unfinished uploads on one connection all buffered)
- Affected: every release up to 0.50.1 (native and LLVM backends; the Go backend is net/http's)
- Shipped in: not yet released

A route that takes its body whole buffered each HTTP/2 request body until it ended, and gave the flow control window back at once, so one connection could hold max_concurrent_streams (250) bodies of up to the body limit (64 MiB) each. The unfinished bodies a connection buffers for streams no handler has yet are now capped together at four times the body limit (XO_HTTP_MAX_BODY, spec/std.md 4.6); the stream that would pass the cap is refused with RST_STREAM REFUSED_STREAM, which tells the client no handler saw it and it may retry. Routes with handle_stream are unchanged (their window comes back as the handler reads).

## 2026-11-13: security: an HTTP/1.1 server reads a slowly sent request head in linear time

- Kind: security
- Commit: this push
- Found by: the std/http security review (decision 0616), an extra finding (no Go advisory: net/http reads a head line by line)
- Test: std/native/llrt/http1_test.xo "a head sent a byte at a time is scanned and parsed a bounded number of times"
- Affected: every release up to 0.50.1 (native and LLVM backends)
- Shipped in: not yet released
- Remote trigger: any client of the server (a denial of service: CPU)

The server looked for the end of an incomplete head from its start and parsed all of it again after every read, so a client sending a head of up to the 1 MiB limit a few bytes at a time cost the server time quadratic in its length. The scan now resumes where the last one ended, and an incomplete head is parsed again only when it has doubled (its early errors are still answered).

## 2026-11-13: security: an HTTP/1.1 server never sends a 100 Continue after the response head

- Kind: security
- Commit: this push
- Found by: the std/http security review (decision 0616), against GO-2021-0224 (CVE-2020-28362 class: net/http's canWriteContinue)
- Go reference: net/http server.go (expectContinueReader.Read writes 100 Continue only while canWriteContinue; WriteHeader clears it)
- Test: golden http_continue_after_head (fails before the fix: a 100 Continue followed the 200 head)
- Affected: every release up to 0.50.1 (native and LLVM backends; full duplex and streamed responses that read the body after their head)
- Shipped in: not yet released
- Remote trigger: any client sending `Expect: 100-continue` to a route whose response head goes out before it reads the body.

A handler whose response head went out before it read an `Expect: 100-continue` body made the server write `HTTP/1.1 100 Continue` after the final head, where the client reads it as body bytes. Writing the head now ends the chance of a 100 Continue, as in net/http; the change of state is atomic, and a head waits for a 100 Continue already being sent (net/http's writeContinueMu), since a full duplex handler may read and write from two tasks.

## 2026-11-13: security: CONNECT ends HTTP on its connection: the server after any answer, the client after sending it, and ReverseProxy refuses it

- Kind: security
- Commit: this push
- Found by: the std/http security review (decision 0616), against GO-2026-6605 (CVE-2026-56866), with Go 1.26.11's server rule for GO-2026-6613
- Go reference: go1.26.11 net/http server.go (closeAfterReply after any CONNECT; a 2xx answer without Content-Length, Transfer-Encoding or Connection), transport.go (no keep-alive after CONNECT), httputil reverseproxy.go (CONNECT answered 405)
- Test: golden http_request_guards (a GET after a CONNECT is not read; ReverseProxy answers CONNECT 405 on arm64, llvm and the Go backend); golden http_client_connect_pool (a request after a CONNECT, sent or opened as a stream, dials anew on every backend)
- Affected: every release up to 0.50.1; the server part native and LLVM backends, the client and ReverseProxy parts all backends (the Go backend's client sets Close on a CONNECT, which Go 1.26.1's transport did not do itself)
- Shipped in: not yet released

A CONNECT request turns its connection into something other than HTTP, but the client pooled the connection after the answer, and ReverseProxy forwarded CONNECT upstream like any request; a body sent with it could then be read as a request of its own (a desynchronization class). The client now closes the connection after a CONNECT whatever the answer, with send and with open, ReverseProxy answers CONNECT 405 as Go 1.26.11 does, and the server, which since the entry below closed only after a 2xx answer, closes after any answer to CONNECT and sends a 2xx one without framing or Connection fields.

- Remote trigger: an upstream or client in a position to desynchronize the connection; a program sending CONNECT itself.

## 2026-11-13: fix: an HTTP/2 server closes a failed connection at its linger deadline while input still arrives

- Kind: fix
- Commit: this push
- Found by: the std/http security review (decision 0616), against GO-2022-0969 (CVE-2022-27664)
- Go reference: net/http h2_bundle.go goAwayTimeout (the connection closes a second after a GOAWAY of an error)
- Test: none fails before the fix: a loopback client flooding a failed connection still left the server idle reads, where the deadline was already checked (arm64, llvm); the review records the run
- Affected: none shown
- Shipped in: not yet released

After a GOAWAY of an error the server drops what the peer sends and closes a second later, but it checked that deadline only when a read found nothing. The deadline is now also checked after each read that brought data, so a peer that never pauses cannot hold the connection open.

## 2026-11-13: security: serve_content ignores a Range header of more than 200 ranges

- Kind: security
- Commit: this push
- Found by: the std/http security review (decision 0616), against GO-2026-6609 (CVE-2026-78667)
- Go reference: net/http fs.go parseRange (defaultMaxContentRanges 200, counted before parsing), go1.26.11
- Test: std/native/http_test.xo "a Range header of more than 200 ranges is ignored, as Go 1.26.11"
- Affected: releases with serve_content and file_server (decision 0306) up to 0.50.1, native and LLVM backends; the Go backend gets Go's limit with its move to go1.26.11
- Shipped in: not yet released

`serve_content` (and so `file_server`) parsed every range of a Range header, so one request could ask for a header's worth of tiny ranges and make the server parse and answer them all (a denial of service class). A header of more than 200 ranges is now ignored and the whole content is served, as in Go 1.26.11.

- Remote trigger: any client of `serve_content` or `file_server`.

## 2026-11-13: security: ReverseProxy forwards only the query its parse and rewrite hook saw

- Kind: security
- Commit: this push
- Found by: the std/http security review (decision 0616), against GO-2022-1038 (CVE-2022-2880) and GO-2026-4976 (CVE-2026-39825)
- Go reference: net/http/httputil reverseproxy.go cleanQueryParams (applied before Rewrite), go1.26.1
- Test: golden http_request_guards (a query with a bad escape and a semicolon reaches the upstream as the parsed `a=1`, a clean one as written; arm64, llvm and the Go backend)
- Affected: releases with http.reverse_proxy (decision 0404 stage D) up to 0.50.1, all backends
- Shipped in: not yet released

`ReverseProxy.serve` forwarded the incoming raw query as written while its parse matched what the request's query showed. Parameters the parse dropped (a semicolon, a bad percent escape, or any past the 10,000 parameter limit) were invisible to the rewrite hook but still reached the upstream (a query smuggling class). Such a query is now rebuilt from the parsed parameters, as Go's cleanQueryParams does; a clean query is still forwarded as written, keeping repeated keys and their order, as Go forwards it. `url.query` holds each key's first value only, so a hook that checks a key which may repeat reads `query_all` or `raw_query` (Lang 2's review; ReverseProxy's documentation says so).

- Remote trigger: any client of a server using `reverse_proxy`.

## 2026-11-13: security: a native server closes the connection after a 2xx answer to CONNECT

- Kind: security
- Commit: this push
- Found by: the std/http security review (decision 0616), against GO-2026-6613 (CVE-2026-94439)
- Go reference: GO-2026-6613 (net/http server kept reading requests after a 2xx CONNECT answer), fixed in go1.26.9
- Test: golden http_request_guards (a GET after a CONNECT answered 200 is not read; the Go backend's go1.26.1 still reads it, want_go.txt)
- Affected: every release up to 0.50.1, native and LLVM backends
- Shipped in: not yet released

A 2xx answer to an HTTP/1 CONNECT turns the connection into a tunnel for the client and any proxy in front, but the server went on reading HTTP requests from it. A proxy that forwarded the tunnel's bytes could have them served as requests (a request smuggling class). The server now closes the connection after such an answer; std/http has no server side tunnel, so nothing is lost.

- Remote trigger: any client, when a handler answers CONNECT with 2xx.

## 2026-11-13: security: an HTTP/2 client drops a response's connection specific fields and a malformed Content-Length

- Kind: security
- Commit: this push
- Found by: the std/http security review (decision 0616), against GO-2026-6610 (CVE-2026-78660)
- Go reference: net/http/internal/http2 transport.go (go.dev/cl/835145): connHeaders deleted from the response, Content-Length kept once only when every value agrees and parses
- Test: std/native/llrt/http1_test.xo "an HTTP/2 response's framing fields as Go's fix for GO-2026-6610 leaves them"; the HTTP/2 and reverse proxy goldens
- Affected: releases with the native HTTP/2 client up to 0.50.1, native and LLVM backends
- Shipped in: not yet released

An HTTP/2 response's Connection, Keep-Alive, Proxy-Connection, Transfer-Encoding and Upgrade fields, and a Content-Length of any form, reached the program as received. A reverse proxy could then write a malformed Content-Length to an HTTP/1 client of a HEAD request or of HTTP/1.0 (a response smuggling class). These fields are now dropped, as in Go; Content-Length stays only as one run of digits all its values agree on.

- Remote trigger: an HTTP/2 server the program talks to. A Content-Length is kept only as Go's ParseUint(v, 10, 63) takes it: digits whose value is at most 2^63 - 1 (Lang 2's review: a longer run of digits passed before).

## 2026-11-13: security: a streamed request's declared trailer names are capped at 64

- Kind: security
- Commit: this push
- Found by: the std/http security review (decision 0616), against GO-2026-6603
- Go reference: GO-2026-6603 (HTTP/2 server memory from Trailer declarations), fixed in go1.26.9
- Test: std/native/http_test.xo "a request's declared trailer names are each kept once, at most 64 of them"
- Affected: releases with handle_stream trailers (decision 0433) up to 0.50.1, native and LLVM backends
- Shipped in: not yet released

A `handle_stream` route's body kept one string per name in the request's `Trailer` field. Over HTTP/2, many streams could each declare a header list's worth of short names, costing far more memory than the header bytes (a denial of service class). Names are now kept once each and at most 64 of them, the most trailer fields a body may end with, and the scan stops there.

- Remote trigger: any client of a server with a `handle_stream` route.

## 2026-11-13: security: an HTTP/2 server's SETTINGS and connection WINDOW_UPDATE frames no longer walk every open stream

- Kind: security
- Commit: this push
- Found by: the std/http security review (decision 0616), against GO-2026-6611 (CVE-2026-78669)
- Go reference: Go's fix (go.dev/cl/847186, go1.26.9): an INITIAL_WINDOW_SIZE change is O(1), each stream's window relative to it, an overflow a FLOW_CONTROL_ERROR when next used
- Test: the HTTP/2 goldens (http_h2_config, http_h2c, http_h2c_sniff, http_stream_body_h2, http_duplex_h2tls, grpc_calls and the other http_ and grpc_ programs, 52 of them on arm64 and llvm)
- Affected: every release with the native HTTP/2 server up to 0.50.1, native and LLVM backends
- Shipped in: not yet released

Each SETTINGS frame with SETTINGS_INITIAL_WINDOW_SIZE updated every open stream's send window, and each SETTINGS frame and connection WINDOW_UPDATE then visited every open stream again to wake it. A peer with many streams open could make each small frame cost work for every stream (a denial of service class). A stream's window is now kept relative to the peer's initial window, so the change is one store; a window it pushes past 2^31-1 fails the connection with FLOW_CONTROL_ERROR when next used, as in Go. Streams are woken only when one stopped on the window that grew.

- Remote trigger: any HTTP/2 client.

## 2026-11-13: security: an HTTP/2 server decodes a header field cut across CONTINUATION frames once

- Kind: security
- Commit: this push
- Found by: the std/http security review (decision 0616), against GO-2023-1571 (CVE-2022-41723)
- Go reference: golang.org/x/net/http2/hpack Decoder.parseFieldLiteral (the fix: both strings whole before either is decoded)
- Test: std/native/llrt/hpack_test.xo (a block fed at every cut decodes as the whole block; a field cut before its value ends decodes nothing of it)
- Affected: every release with the native HTTP/2 server up to 0.50.1, native and LLVM backends (the Go backend serves with Go's net/http)
- Shipped in: not yet released

The server's HPACK decoder decoded a literal field's name before it knew the value had arrived, and decoded it again with each later fragment while the value was incomplete. The CPU cost of a header block could grow with the square of its size (a denial of service class). A field is now decoded only once both of its strings are whole.

- Remote trigger: any HTTP/2 client.

## 2026-11-13: behavior: a server answer that closes after a request body too large: Connection: close after the handler's fields, and none on HTTP/1.0, as Go 1.26.11

- Kind: behavior
- Commit: this push
- Found by: TestHTTPParseDiff and TestHTTP1ParseDiff against the go1.26.11 oracle (every host on 1.26.11 since 2026-11-13)
- Go reference: net/http chunkWriter.writeHeader and requestTooLarge (go1.26.11: requestTooLarge no longer sets the handler's Connection header; a closing HTTP/1.0 answer sets setHeader.connection = "")
- Test: TestHTTPParseDiff (POST HTTP/1.0 keep-alive with Content-Length 1000000), TestHTTP1ParseDiff (OPTIONS * with Content-Length 5000); arm64, llvm

## 2026-11-13: behavior: a response with a malformed header field: the error quotes the line, as Go 1.26.2 and later

- Kind: behavior
- Commit: this push
- Found by: the Go 1.26.1 to 1.26.11 toolchain trial (owner item 9, 2026-11-13: TestGoldenGoParity's http_client_responses differed)
- Go reference: net/textproto readMIMEHeader as net/http's ReadResponse reports it (go1.26.11; go1.26.1 printed the text raw)
- Test: goldens http_client_responses (want.txt) and TestHTTP1ClientDiff (testdata/http1_client.txt) rewritten from the Go backend on go1.26.11; arm64, llvm, Go parity
- Timing: none (error text only)
- Shipped in: not yet released

The native client's `malformed MIME header line` and `malformed MIME header initial line` errors show the field line as a Go quoted string (`"X-Ctl: a\x01b"`), as net/http's do with Go 1.26.2 and later.
## 2026-11-13: security: a client never writes a CR or LF from with_host or User-Agent into the request

- Kind: security
- Commit: this push
- Found by: the std/http security review (decision 0616), against GO-2023-1878 (CVE-2023-29406)
- Go reference: net/http request.go (Request.write: httpguts.ValidHostHeader, and headerNewlineToSpace on User-Agent), go1.26.1
- Test: golden http_request_guards (arm64 and llvm want.txt; on the Go backend Go's net/http sends an empty Host and rejects the User-Agent value, want_go.txt)
- Affected: every release up to 0.50.1, native and LLVM backends (the Go backend sends with Go's net/http)
- Shipped in: not yet released

A request's Host (`Request.with_host`) and its `User-Agent` value were written into the HTTP/1 request head as given, outside the header writer that turns CR and LF into spaces. A program that put untrusted text into either could have it add header fields or a second request. A Host with a byte outside Go's ValidHostHeader set now fails the request with `http: invalid Host header` before it connects (Go sends an empty Host on plain HTTP/1, for Unix socket addresses this client does not dial). A User-Agent's CR and LF become spaces and the value is trimmed, as in Go.

- Remote trigger: none by itself; the program must pass untrusted text to `with_host` or the User-Agent header.

## 2026-11-13: security: a native server closes the connection after a request with both Transfer-Encoding and Content-Length

- Kind: security
- Commit: this push
- Found by: the std/http stress program (decision 0616, std/evidence/http/stress); the lead asked for RFC 9112's rule over Go's (2026-11-13)
- Go reference: net/http server.go (readTransfer drops Content-Length under chunked and keeps the connection open), go1.26.1; RFC 9112 6.3 item 3 (the server MUST close the connection after responding)
- Test: golden http_request_guards (a GET pipelined after the request is not answered; arm64 and llvm want.txt, the Go backend want_go.txt); internal/stdtest/httpsmuggle_test.go keeps the framing equal to Go's
- Affected: every release up to 0.50.1, native and LLVM backends; the Go backend serves with Go's net/http and still keeps the connection open
- Shipped in: not yet released

A request with both `Transfer-Encoding: chunked` and `Content-Length` was already framed by chunked alone, as in Go. The server now also answers it with `Connection: close` and closes the connection, so any bytes after the request on that connection are never read as another request. A front proxy that framed the same request by its Content-Length could otherwise have the bytes it took as body served as a request of their own (request smuggling).

- Remote trigger: any client.
- Behavior change: a client that pipelines requests after such a request gets no answer to them and must reconnect.

## 2026-11-12: feature: Client.with_jar and CookieStore

- Kind: feature
- Commit: this push
- Found by: decision 0617 B (the owner's high group of std features, 2026-11-12; the owner chose the installer pays, 2026-11-12)
- Go reference: net/http client.go (Client.Jar: send adds Jar.Cookies to each request and stores resp.Cookies() with Jar.SetCookies, redirects included), go1.26.1
- Test: golden http_client_jar (a redirect's cookie reaching the next hop, a final response's kept, the request's own Cookie first, no jar no cookies, a program's own store; arm64, llvm, Go parity)
- Timing: none (one store call each way a hop, only with a jar)
- Shipped in: not yet released

`with_jar(store, clock)` gives the client a `CookieStore` (`cookiejar.Jar`, or a program's own). Each hop asks the store for its URL's cookies, appended after the request's own `Cookie` field, and stores the response's `Set-Cookie` fields at the clock's time. Go's redirect rule that drops a request's own Cookie field when the jar changed a cookie of the same name is not ported: the request's own field is kept on same host hops and dropped on cross host ones, as without a jar.

## 2026-11-12: feature: Client.with_proxy and with_proxy_for; fix: the Go backend no longer reads HTTP_PROXY implicitly

- Kind: feature, fix
- Commit: this push
- Found by: decision 0617 B (the owner's high group of std features, 2026-11-12); the Go backend's implicit proxy found while porting it
- Go reference: net/http transport.go (connectMethod, dialConn's CONNECT and its error texts, Proxy, OnProxyConnectResponse), go1.26.1
- Test: golden http_client_proxy (a forward proxy over raw TCP: absolute form with Proxy-Authorization, a redirect asking the chooser again, https through CONNECT with the certificate verified, a chooser's None, Net.only refusing the proxy, a refused CONNECT; arm64, llvm, Go parity)
- Timing: none (a chooser call a request; the tunnel is one round trip on a new connection)
- Shipped in: not yet released

Plain http goes to the proxy in absolute form, https through a CONNECT tunnel with TLS to the server inside; an attenuated Net must admit the proxy; tunnels pool by target, proxy and credentials. The Go backend's transports were clones of Go's DefaultTransport, which reads HTTP_PROXY, HTTPS_PROXY and NO_PROXY on its own, so a Go backend program went through a proxy the native one did not use and Net.only never saw; now neither backend uses a proxy unless the program sets one.

## 2026-11-11: feature: Url user and password, display and redacted, Basic auth from the URL

- Kind: feature
- Commit: this push
- Found by: the std audit of 2026-11-11 (the url.Userinfo rows of scripts/stdaudit/map.txt), decision 0606
- Go reference: net/url url.go parseAuthority, Userinfo.String, URL.Redacted, shouldEscape (encodeUserPassword); net/http client.go send (go1.26.1)
- Test: the httpurl corpus (Go's URL.User, User.String and Redacted on the userinfo of Go's url_test.go inputs and 600 random ones, go, arm64, llvm); golden url_userinfo (display, redacted, the Authorization header a client sends; arm64, llvm, Go parity)
- Timing: none (a parse of the userinfo only when the URL has one)
- Shipped in: not yet released

http.Url gains user and password as defaulted fields, so every literal still compiles; parse_url used to drop them. A URL's password now reaches display: log redacted() instead. A client request to a URL with a user sends Authorization: Basic unless it sets its own. An empty userinfo (http://@host) reads as none, so display writes no "@" there where Go's String does.

## 2026-11-11: feature: file_server(fs, strip), Go's FileServer over an Fs

- Kind: feature
- Commit: this push
- Found by: std audit 2026-11-11 gap 10 (decision 0607)
- Go reference: net/http fs.go FileServer, FileServerFS, StripPrefix (go1.26.1)
- Test: http corpus FS_TREE and FS_REQS (35 requests: files, HEAD and POST, missing paths, directories with and without index.html, index.html and trailing slash redirects with the query, paths through a file, `..`, escaped and odd names, If-Modified-Since on files and listings, ranges, strip) against Go's FileServerFS over the same tree, on go, arm64, llvm
- Shipped in: not yet released

A handler for `Router.handle`. The Fs is the sandbox, so `os.fs.sub(dir)`,
an attenuated Fs or `embed.Dir.fs()` serves only what it holds. Files are
read whole and passed to serve_content until a streaming serve_content
exists. The Go backend runs net/http's FileServer over an http.FileSystem
that reads through the Fs; native code is a port of serveFile and dirList.

## 2026-11-11: feature: cross_origin, CrossOrigin.check and middleware (Go's CrossOriginProtection)

- Kind: feature
- Commit: this push
- Found by: std audit 2026-11-11 gap 14
- Go reference: net/http/csrf.go (go1.26.1)
- Test: http corpus TRUST (12 origins), CHECKS (500 requests over methods, Hosts, paths, Sec-Fetch-Site and Origin, with trusted origins and bypass patterns) and DENIED, from Go, on go, arm64, llvm
- Shipped in: not yet released

Go's protection as a value: `cross_origin(trusted, bypass)` checks the
origins as AddTrustedOrigin does (its errors), `check(req)` is Check
(its texts), and `middleware()` is Handler with http.Error's 403. Bypass
patterns are Router patterns. There is no SetDenyHandler: a program that
wants its own response calls `check` in its own middleware. The Go backend
runs net/http's CrossOriginProtection; native code is a port.

## 2026-11-11: fix: Request.host() is with_host's host on native backends

- Kind: fix
- Commit: this push
- Found by: the CrossOrigin corpus (native and Go disagreed)
- Go reference: Request.Host
- Test: http corpus, the host() line of the CrossOrigin test (go, arm64, llvm)
- Shipped in: not yet released

`r.with_host(h).host()` gave the Host the request came with ("" for one
built with new_request) on arm64, amd64 and LLVM, and `h` on the Go
backend. It is now `h` everywhere, as Go's Request.Host.

## 2026-11-11: behavior (Go backend): query_all reads the query with std/url's port

- Kind: behavior
- Commit: this commit
- Found by: decision 0597's url follow-up (the Go backend still parsed queries with Go's net/url)
- Go reference: net/url ParseQuery, URL.Query
- Test: golden http_form on arm64, LLVM and the Go backend (TestGoldenGoParity)
- Shipped in: not yet released

On the Go backend `Request.query_all` split the raw query with Go's `url.ParseQuery`, a map, and kept invalid UTF-8 as `%XX` afterward, so two keys that read as one name (`%FF` and `%25FF`) came back in map order. It now unescapes each pair in order with std/url's `query_unescape`, as the native backends do, so every backend gives the same values in query order.

## 2026-11-11: docs: the header says what the module is for

- Kind: docs
- Commit: this commit
- Found by: pkg.xolang.org and xolang.dev showed `xo doc std`'s summary of std/http as a spec reference or a note about the file (the Xo Lead, 2026-11-11)
- Go reference: none
- Test: cmd/xo TestDocStdSummaries
- Shipped in: not yet released

The header comment of std/http.xo opens with a sentence that says what the module is for, which `xo doc std` lists as its summary. No declaration changed.

## 2026-11-10: behavior (Go backend): BodyReader.multipart reads with std/multipart's Reader

- Kind: behavior
- Commit: this push
- Found by: decision 0597 part 3 (one multipart implementation)
- Go reference: mime/multipart Reader.NextPart and Part.Read (go1.26.1)
- Test: goldens http_multipart_stream, http_form, multipart_reader and multipart_basic on the Go backend (TestGoldenGoParity)
- Shipped in: not yet released

On the Go backend `PartReader` wrapped Go's mime/multipart.Reader; it now runs std/multipart's
`Reader`, generated into std/rt (decision 0597), as the native backends do, so a part's head, its
data and the error texts are the same on every backend.

## 2026-11-10: refactor: integer bounds as `Int.max` and `Int.min` (decision 0587 point 4)

- Kind: refactor
- Commit: this commit
- Found by: decision 0587 point 4 (std's literal bounds become the type constants)
- Go reference: none (no behavior change; Go's math.MaxInt64 and MinInt64)
- Test: the std module tests and goldens of the gate; the self build

`9223372036854775807` and `-9223372036854775807 - 1` in std/http's code are
`Int.max` and `Int.min`; masks, Float literals, comments and texts keep their
digits. No behavior change.

## 2026-11-10: fix (Breaking): Router matches as Go 1.22's ServeMux: trailing slashes, path cleaning, HEAD

- Kind: fix
- Commit: this commit
- Found by: an owner tester (via the lead, 2026-11-10): `GET /healthz` also answered `/healthz/` and `/healthz///`, and HEAD on a GET route was a 405
- Go reference: net/http ServeMux (go1.26.1, checked with httptest on linux/arm64)
- Test: internal/stdtest corpus http, "Router matches as Go 1.22's ServeMux does" (10 pattern sets x 4 methods x 39 targets from Go's ServeMux)
- Timing: none (a route table of a few entries; the pick is one pass as before)
- Shipped in: not yet released
- Breaking: a program that relied on the slash leniency. `GET /x` no longer matches `/x/` (404); a pattern ending in `/` (`/dir/`) is now a subtree route (matches everything below it and redirects `/dir` to `/dir/`); `/f/{p...}` no longer matches `/f` (it redirects to `/f/`); `{name}` no longer matches an empty segment; a path with `//`, `.` or `..` is answered with a 307 to the cleaned path. Migration: register the slash-free pattern for the exact path and a `/dir/` pattern (or `{p...}`) for a tree; use `{$}` for "the root only".

`Router` now follows Go's matching rules: the request path is cleaned (307 to the cleaned path, query kept, a trailing slash kept); a path with no exact match whose slash form matches a subtree or rest route exactly is a 307 to the slash form; patterns ending in `/` are subtrees and `{$}` is the exact slash path; a `GET` route also takes `HEAD`; a 405's `Allow` is sorted, lists `HEAD` after `GET`, and also counts the routes of the path with a trailing slash. Go 1.26's redirects are 307 (Go 1.22 answered 301). Documented in spec/std.md 4.1 together with two behaviors that already matched Go: `req.json()` does not check Content-Type, and a duplicate JSON key takes the last value.

## 2026-11-09: fix: http.redirect's body ends with two newlines, as Go's does

- Kind: fix
- Commit: this commit
- Found by: Xo Lang 3 (the polarbearblog port's byte for byte diff against the Go app, a GET redirect)
- Go reference: net/http Redirect: `fmt.Fprintln(w, body)` with a body that already ends in "\n" (go1.26.1, checked with httptest)
- Test: std/native/http_test.xo "redirect" case (Found</a>.\n\n)
- Shipped in: not yet released

`http.redirect` for a GET wrote `<a href="URL">Found</a>.` and one newline; Go writes two. A client or test that compared the body bytes
saw the difference; headers were already equal.

## 2026-11-08: fix: under FakeNet, a client request reaches a bare Listener's accept loop

- Kind: fix
- Commit: this push
- Found by: Xo Azure 1 (aws repo docs/reports/xo-feedback.md, the test browser; lead, 2026-11-08)
- Go reference: net/http Client over net.Listen (a real socket); httptest-style servers
- Test: std/native/http_test.xo "a client request in a test reaches a bare Listener's accept loop" (was XO-F001 deadlock: every task is blocked)
- Timing: none (test world only)
- Shipped in: not yet released

In a test (FakeNet), `Client.send` and `get` waited for a router on a listener nobody served, as a real backlog does, so a request to a raw `Net.listen` accept loop deadlocked while a raw `Net.dial` worked. A listener a task accepts on (and no http.server serves) now takes the request as HTTP/1.1 over the fake TCP: `Connection: close`, the body sent whole, the response read to the end of the connection (Content-Length, chunked and close delimited bodies). Redirects, compression and streamed request bodies are not followed on this path. http.server listeners, FakeNet.handle and fail routes are unchanged.

## 2026-11-07: fix: a native server on Linux answers pipelined requests whose sender closed its side right after them

- Kind: fix
- Commit: this push
- Found by: internal/llvm TestHTTP1ParseDiff hung on linux/arm64 hosts (2026-11-07, at main and on every branch since 2026-11-06): each connection waited out its sender's 30 second deadline
- Go reference: internal/poll fd_unix.go Read (Go reads first, so it never waits here)
- Test: internal/llvm TestHTTP1HalfClose (64 half-closed connections of pipelined requests); TestHTTP1ParseDiff
- Shipped in: not yet released
- Affected: 0.43.0 (a seed, not a public release; 0.41.0, the latest public release, predates it), native backends on Linux; introduced by 1c9bc8e5f
- Known issue: 0.43.0, a native server on Linux can leave pipelined requests (or a request followed by more bytes) unanswered when the sender half-closes right after sending them

The read-skip of 2026-11-06 (the read after one that drained the socket waits for readiness first) waited forever when the requests and the sender's FIN came under one poller edge: the short read took the requests, the server answered the ones it had buffered, and the end of stream it left had no new edge to announce it. The poller now keeps a hangup mark per descriptor (EPOLLRDHUP, EPOLLHUP, EPOLLERR), and a read after a draining one does not wait once the peer has closed.

## 2026-11-07: fix: a FakeNet handler's http.stream body reaches Client.send and Client.open, and open reads it flush by flush

- Kind: fix
- Commit: this push
- Found by: aws-19: azcore's server-sent events tests: a `FakeNet.handle` handler answering `http.stream(...)` gave an empty body through both `send` and `open` (natively)
- Go reference: net/http/httptest ResponseRecorder for the whole body; an http.Response.Body that returns each flushed write for open
- Test: cmd/xo TestFakeNetStreamBody (Go backend, arm64, LLVM)
- Shipped in: not yet released

Natively, `send` now runs a `handle` handler's `http.stream` function into the body, as a served router's call already did; before, the body came back empty. On both backends `Client.open` under FakeNet now runs the handler in its own task for every request, not only one with a source body, so a streamed response reaches `read` flush by flush: a test can read one event before the handler writes the next. A request's whole body reaches the handler as its body, read whole or as a stream.

## 2026-11-06: fix: escaped_path escapes a path written with a space on the Go backend

- Kind: fix
- Commit: this push
- Found by: aws-19 (azblob: 4 of 27 tests failed on `--backend=go` for a blob name with a space)
- Go reference: net/url URL.setPath and URL.EscapedPath (RawPath used only while validEncoded) (go1.26.1)
- Test: internal/llvm/testdata/golden/http_escaped_path (every backend, Go parity)

On the Go backend, `http.new_request("GET", "http://h/x/y z.txt").escaped_path()`
returned `/x/y z.txt`: the request kept Go's RawPath, the path as
written, even when it is not a valid encoding. It now keeps RawPath only
when Go's EscapedPath would use it, so `escaped_path()` is `/x/y%20z.txt`,
as on the native backends; a valid escape the decoded path loses
(`a%2Fb`) is still kept. The client already sent the escaped path.

## 2026-11-06: feature: http.status_text, net/http's StatusText

- Kind: feature
- Commit: this push
- Found by: aws-19: azcore carries a copy of net/http's StatusText table because std/http has no public one
- Go reference: net/http StatusText (status.go go1.26.1)
- Test: testdata/corpus/http "http.status_text is Go's http.StatusText" (every code from -1 to 700 against Go's table; native, Go backend)
- Shipped in: not yet released

`http.status_text(code)` is the reason phrase of a status code (`Not Found` for 404, `I'm a teapot` for 418), empty for a code Go does not name. It is the table the native server writes in the status line.

## 2026-11-06: perf: a native server on Linux skips the empty read after one that drained the socket

- Kind: perf
- Commit: this push
- Found by: /proc/PID/io syscall counts of bench/perf/http's server under loadgen on linux/arm64 host 1 (2026-11-05): native 2.0 reads and 1 sendto per keep-alive request, one read of them empty (strace and perf tracepoints are not available on host 1). Go: 2.37 reads, 1.05 writes
- Go reference: internal/poll fd_unix.go Read: Go also reads first and waits on EAGAIN; it does not skip (its netpoller is edge triggered the same way), so this goes past Go
- Test: TestGoldenRun http_* (arm64), internal/llvm TestGolden http_*, internal/selfhost TestSelfhostWasm (http_*, conc_*)
- Timing: xo bench --ab on linux/arm64 host 1, bench/perf/httpkeep (32 clients, 30,000 requests each): wall 0.993 (0.98 to 1.005), CPU 0.988 (0.985 to 1.004), within noise; max RSS 0.98, better. Not a speedup claim. read(2) calls per request (/proc/PID/io syscr, bench/perf/http's server under loadgen, 32 keep-alive workers, 3 alternating rounds of 5 s on host 1): 1.96 to 2.00 before, 1.001 to 1.003 after
- Shipped in: not yet released

A plain HTTP/1.1 read that returned fewer bytes than it asked for drained the socket, so the next read on the connection found nothing and returned EAGAIN before waiting for readiness. On Linux the read after a draining one now waits first when no readiness edge came since (the poller is edge triggered and keeps an untaken edge in the descriptor). A parked connection clears the mark, since it resumes on an edge its resumer took. A pipelined response is still sent at once (decision 0074's 2026-11-06 addendum).

## 2026-11-06: fix: a half-close, read or write no longer hits another connection that reused the fd's number

- Kind: fix
- Commit: this push
- Found by: TestGoldenTasks http_reverse_proxy_ws under the held Linux skip-read change (a 502 on a proxied WebSocket dial)
- Go reference: none (Go's net.conn holds its fd through fd.incref for each operation)
- Test: the golden http_reverse_proxy_ws under the skip-read change, a 50-round repro of its two dials (0 failures with the fix); gotchas/shutdown-of-a-reused-fd.md
- Timing: none (one lock pair per shutdown)
- Shipped in: not yet released

## 2026-11-06: perf: a header name's case insensitive compare steps 8 bytes at a time

- Kind: perf
- Commit: this push
- Found by: the SIMD survey of std's byte paths (bench/results/simd-survey-2026-11-06.md)
- Go reference: net/textproto, strings.EqualFold (go1.26.1)
- Test: std/native/llrt/scan_test.xo "fold_eq_words and case_run match a byte loop at each byte", `xo test --std-module=native/llrt/http1`
- Timing: the same loop as Str.equal_fold: bench/perf/simdscan fold_32 0.212 of main's wall on linux/arm64
- Shipped in: not yet released

std/native/llrt/http1.xo equal_fold compares 8 byte words while they are ASCII and equal with capitals lowered (scan.xo fold_eq_words).

## 2026-11-06: feature: Request.param_opt, and a second FakeNet.handle replaces the first

- Kind: feature
- Commit: this commit
- Found by: Xo Azure 1 (azure repo 4bbedcc)
- Go reference: net/http Request.PathValue returns "" for a missing name (Xo's param faults; param_opt is the Option form)
- Test: native/http "a second FakeNet.handle for the same pattern replaces the first, and param_opt answers None for a missing segment"

`req.param_opt(name) -> Str?` answers None where `req.param(name)` faults (the route has no such segment).
`FakeNet.handle` with the same method, URL prefix and path pattern as an earlier one now replaces it (last wins); before,
the first kept serving and the second was ignored silently. Go's ServeMux panics on a duplicate pattern; Xo takes last wins
because a test overrides a default handler it set up earlier, and a panic would make that impossible.

## 2026-11-06: behavior: the client sends header names as the caller stored them

- Kind: behavior
- Commit: this push
- Found by: aws-19 (Azure Storage): `x-ms-meta-project` went out as `X-Ms-Meta-Project`, so Azure stored the metadata name `Project`
- Go reference: net/http writes a Header map's keys as they are; only Header.Set and Add canonicalize
- Test: internal/llvm/testdata/golden/http_client_raw_header_names (the request head on the wire, native, LLVM and Go)
- Shipped in: not yet released

`Client.send` and `Client.open` no longer canonicalize the names of `req.headers`. `http.new_request(..., headers)` and `req.with_header` still canonicalize (Go's Header.Set); to send a non-canonical name, assign the map directly: `req with {headers: {"x-ms-meta-project": "a"}}`. The redirect rule that drops Authorization, Cookie and the like still matches names in canonical form, as Go's shouldCopyHeaderOnRedirect.

## 2026-11-06: fix: FakeNet and Router match escaped paths on their unescaped segments

- Kind: fix
- Commit: this push
- Found by: aws-19 (Azure Storage blob names with a slash, a space or a non-ASCII character)
- Go reference: net/http ServeMux (go1.22 patterns): the path is split on its escaped slashes, each segment unescaped, and a pattern's literal segments are unescaped too
- Test: std/native/http_test.xo "FakeNet matches a pattern on the escaped path's unescaped segments"
- Shipped in: not yet released

A client request's `path` is decoded, so `/obj/x%2Fy` split into two segments and matched nothing. FakeNet now splits the escaped path (`raw_path` when set) before unescaping each segment, so `{name}` binds `x/y`; a pattern written `/dir/a%20b/leaf` matches the segment `a b` (the same unescaping now applies to a Router's patterns).

## 2026-11-05: fix: a native server's answers after a stop say Connection: close

- Kind: fix
- Commit: this push
- Found by: the stop-test template (internal/llvm/testdata/golden/http_stop_keepalive): the Go backend's answer to a request in flight at a stop carried Connection: close and native's did not
- Go reference: net/http server.go: Shutdown turns keep-alives off (doKeepAlives false), so a response sets closeAfterReply and adds Connection: close
- Test: internal/llvm/testdata/golden/http_stop_keepalive (arm64, llvm, Go parity): a request in flight at the stop is answered with Connection: close
- Timing: none
- Shipped in: not yet released

A stopping native server already closed each HTTP/1.1 connection after its current response, but the response did not say so, so a client could send its next request on a connection about to close. The response now has Connection: close (and an HTTP/1.0 keep-alive request's response no longer says keep-alive), as net/http's after Shutdown.

## 2026-11-05: fix: a native server stops when a kept-alive connection answered a request just before the stop

- Kind: fix
- Commit: this push
- Found by: the stability program's soak (decision 0520 phase 1): stability/notes did not exit within 20 s of SIGTERM on 5 of 8 two-minute runs; a task dump showed serve waiting with no connection tasks
- Go reference: net/http Server.Shutdown (closes idle connections, then polls until every connection is idle or closed)
- Test: internal/llvm/testdata/golden/http_stop_keepalive (arm64, llvm): four kept-alive connections, a request each, a stop; serve returns before the 2 s grace period and each connection is closed
- Timing: none
- Shipped in: not yet released (the bug: 0.36.0 to 0.40.0; 0.35.0 did not park idle connections)

A kept-alive HTTP/1.1 connection that answered a request waits up to 20 ms on its task before it parks idle (decision 0449). A stop in those 20 ms found it busy and left it open, and the parked-connection resumer, seeing nothing parked, ended; the connection then parked with no resumer and was never closed, so `serve` waited for it until the grace period canceled it, or for ever with none (SIGTERM never stopped such a server: 10 of 10 runs of a one-request repro on 0.36.0, 0.39.0 and 0.40.0). A connection now checks for a stop under the resumer's lock before it parks, and closes instead (gotchas/park-after-stop-check-then-act.md).

## 2026-11-05: perf: a pass-through connection reads into a 32 KiB block, not a 64 KiB one

- Kind: perf
- Commit: this push
- Found by: decision 0506 (bench/results/size-class-waste-2026-11-04.md: a Bytes of n bytes takes n + 24 of a block, so a power of two buffer fell into the class twice its size)
- Go reference: none (Go's size classes are about four per doubling, so its 16 and 32 KiB buffers waste about 1%)
- Test: internal/llvm/testdata/golden/http_reverse_proxy_ws and http_reverse_proxy (arm64, llvm)
- Timing: none (memory: half the block per pass-through direction)
- Shipped in: not yet released

The pass-through (a proxied upgrade or CONNECT) read buffer is 32,736 bytes, which fills the 32 KiB size class; 32,768 took the 64 KiB one.

## 2026-11-05: feature: Request.with_expect_continue sends the head, waits for the 100, then sends the body

- Kind: feature
- Commit: this commit
- Found by: aws-19 (S3 PutObject over 2 MB needs Expect: 100-continue)
- Go reference: net/http Transport.ExpectContinueTimeout, transfer.go (expectContinueReader), persistConn.writeLoop
- Test: internal/llvm/testdata/golden/http_expect_continue (100 then 200, a 401 before any 100 with no body sent, no answer until the timeout, a 103 and a 100; the Go backend, native and LLVM print the same)

`with_expect_continue(timeout = 1s)` sets `Expect: 100-continue` and the wait. The client sends the head alone, then the body after a `100 Continue` or when the timeout passes; a final response first is returned and the body is withheld (the connection closes, as Go does); other 1xx are skipped. Client.send over HTTP/1.1 only: an HTTP/2 request, Client.open and a request without a body send as before. A with_body_reader body is not read until the 100 (or the timeout), and never after a final response. A request that sets the Expect header by hand with with_header still sends its body at once.

## 2026-11-05: fix: an h2c server installed by two threads at once no longer frees the losing table at a wrong address

- Kind: fix
- Commit: (this commit)
- Found by: reading the inlined code of the 2026-11-05 darwin race stress report in srvh2c.table
- Go reference: none (the native runtime's own records)
- Test: none deterministic (the losing side of a race between two first installs); libcmem.xo heap_release's prefix check ends the program if it runs
- Affected: 0.36.0 to 0.39.0, native and LLVM backends: srvh2c.table released its rt.heap_large block at the result instead of the prefix (result minus 8) when another thread published the table first (gotchas/heap-large-prefix-not-malloc-address.md)
- Shipped in: not yet released

## 2026-11-05: security: a native server's response trailers no longer corrupt the runtime's heap

- Kind: security
- Commit: cbe600955
- Found by: the 2026-11-05 race stress (http_h2_config, http_trailers and http_server_limits stopped on TSan's allocator CHECK under `--race`, 24 of 24 runs)
- Go reference: none (the native runtime's own records)
- Test: TestGoldenRun http_trailers, http_h2_config, http_server_limits, which now fail without the fix (libcmem.xo heap_release's prefix check)
- Affected: 0.36.0 to 0.39.0, native and LLVM backends (not the Go backend); introduced by e0444461a (trailers, decision 0433) and 4d83e777b (HTTP/2 settings, decision 0434), 2026-10-30
- Shipped in: not yet released

A server's runtime freed three of its blocks 8 bytes past their start (gotchas/heap-large-prefix-not-malloc-address.md). The free read a stale word as the block's size class, so it could put an offset block on a free list (two later allocations then share memory), write past the thread cache's class table, or unmap the wrong range.

- Remote trigger: a client can trigger it. Every response whose handler calls `BodyWriter.set_trailer` (a streaming handler's trailers, HTTP/1.1 or HTTP/2) freed its trailer block this way when the response's sink was freed, and again for each trailer after the first. A client only has to request such an endpoint; it cannot choose the stale word.
- Settings block: a server built with HTTP/2 settings (`Server.with_h2_config`) freed its settings block the same way once, when the server closed. This is not client-driven.
- Workaround on affected versions: do not set response trailers from native handlers. The settings block costs one bad free at shutdown.

`--race` builds take every block from the C library's malloc, so the same frees made TSan's malloc return misaligned blocks. They are now released at the prefix. `rt.heap_release` of an address that is not 16 byte aligned now ends the program (std/changes/prelude.md).

## 2026-11-04: fix: Client.open of a piped request is full duplex under the fake net

- Kind: fix
- Commit: this push
- Found by: aws-19's issue 9 (a full duplex http.pipe test deadlocked under `xo test`; it passed with `--real`)
- Go reference: net/http's transport sends the body while it reads the response; Go's httptest servers are real
- Test: test_http_fake_duplex (native `xo test` and the Go backend: duplex echo, a client that closes early, a body read to its end first)
- Shipped in: not yet released

Under FakeNet (`os.net.listen` in memory listeners), `Client.open` read the whole request body (`src_whole`) before the handler ran and returned the whole response, so a test whose body writer waits for the response deadlocked. A request with a body source now runs the handler in a task with the source as its request body; the head and a streamed response's writes come back through channels, so the answer to each piece is readable while the pipe stays open. Requests without a body source and FakeNet `handle` handlers are unchanged. On the Go backend `http.pipe` made in a test world is now scheduler visible (its reads and writes wait on a channel instead of a blocking io.Pipe call, which the test scheduler reported as a deadlock), so the exchange stays deterministic under `--seed` and replay.


## 2026-11-04: fix: a request to an IPv6 literal host keeps the brackets in its URL text

- Kind: fix
- Commit: this push
- Found by: aws-19's full duplex repro under `xo test --real` (Url.host holds "::1" without brackets; the client rebuilt `http://::1:45517/echo` and refused it: invalid port)
- Go reference: net/url's URL.String brackets an IPv6 host
- Test: golden http_ipv6_url (every backend; a server on [::1], Client.get of its own address)
- Shipped in: not yet released

`request_url_string` and `url_string_path` (native) and std/rt `urlStringPath` (Go backend) wrote the host as stored. A host containing a colon and no leading bracket is now written in brackets, so `Client.get("http://[::1]:PORT/...")` reaches a listener on `[::1]` instead of failing with `invalid URL`.


## 2026-11-04: security: an HTTP/2 server's header list limit follows with_max_header_bytes

- Kind: security
- Commit: this push
- Found by: an H2Config differential against Go's net/http for the owner's v0.38.1 "not tested yet" list (Xo Lang 2, a raw frame probe on linux/arm64)
- Go reference: net/http's http2 serverConn.maxHeaderListSize (MaxHeaderBytes plus 10 fields of 32 bytes) for SETTINGS_MAX_HEADER_LIST_SIZE, the header block budget and the HPACK string limit
- Test: golden http_h2_max_header_bytes (every backend)
- Shipped in: not yet released

The native HTTP/2 server advertised and enforced a header list of 1,048,896 bytes whatever `with_max_header_bytes` said, so a server limited to 8 KB of headers on HTTP/1.1 took 1 MB per request on HTTP/2. It now uses `with_max_header_bytes` plus 320, as Go: a request whose fields add up past it gets 431, and a single field longer than it ends the connection with GOAWAY COMPRESSION_ERROR. A server with no `with_max_header_bytes` keeps 1,048,896.

## 2026-11-04: fix: the Go backend's "absolute URL required" message keeps a hostless URL as written

- Kind: fix
- Commit: this push
- Found by: full tier 46 (TestGoldenGoParity/http_client): the raw path change (dec3975c9) made the Go backend print `not%20a%20url`, the native backends and want.txt `not a url`
- Go reference: none (Xo's own message)
- Test: TestGoldenGoParity http_client, http_raw_path, http_server_request
- Shipped in: not yet released

requestURLString wrote a URL with no host through EscapedPath; it now
writes the path as given, as the native backends do.

## 2026-11-04: fix: a server finishes a streamed request body under its lock, so a reverse proxy's body sender no longer races the connection

- Kind: fix
- Commit: this push
- Found by: Xo Lang 2's ThreadSanitizer rounds of TestGoldenTasks (2026-11-04): http_reverse_proxy_retry, XO_THREADS=2, a data race of http1.body_next (the proxy's full duplex sender reading the request body, decision 0433) against serve_h1's body_finish closure
- Cause: stream_finish copied the body's finish function out under a read lock and ran it unlocked, while BodyReader.read runs body_next under the body's lock: the server could finish (and move the connection on to its next request) while a read on another task was still in body_next
- Go reference: net/http transfer.go body.Close takes body.mu, the lock Read holds, so a close waits for an in-flight read; a read after it is "http: invalid Read on closed Body"
- Fix: stream_finish runs finish under the body's lock, then drops the body's reader; a later read gets net/http's closed body error (never the next request's bytes, never a silent end that would cut a proxied upload short)
- Tests: TestGoldenRun http_reverse_proxy*, http_stream*, http_duplex*, http_trailers, http_h2c*, http_server*, http_pipe and others (21); ThreadSanitizer rounds of http_reverse_proxy_retry
- Severity (2026-11-05, golden http_proxy_body_next_race): a reverse proxy forwarded the next pipelined request's bytes upstream as part of another client's upload in 17 of 27 runs before the fix on linux/arm64 (plain builds at 2 and 8 threads too, not only --race), and left second requests unanswered in 20; 0 of 27 on main. Present in v0.36.0 to v0.38.1.

## 2026-11-04: fix: the client sends a request's path as written (RawPath), and http://host/ reaches a listener on host:80

- Kind: fix
- Commit: this push
- Found by: aws-19 (2026-11-04, docs/reports/xo-issues.md entry 2): `new_request` kept only the decoded path and the client escaped it again, so `/bucket/a%20b%2Bc` went out as `/bucket/a%20b+c` (a SigV4 signature over the first fails on the second), and `%2F` became `/`
- Cause: url_parts returned the decoded path only; Request.raw_path was set by the server side alone
- Go reference: net/url URL.setPath (RawPath kept when escaping Path gives something else) and URL.EscapedPath (RawPath returned while validEncoded and it unescapes to Path), which Client.Do writes as the request target
- Test: golden http_raw_path (arm64, LLVM, Go backend; want.txt is Go's own output through net/http for the ten paths: %2B, %2F, %2f, %20, +, UTF-8 escapes, `//`, `!*'()`, %25, %7E, a raw space); http_server_request's want.txt follows Go (`/esc/a%20b%2B`); golden test_http_default_port (`xo test`, both backends)
- Timing: none (a path kept as a string; the default form still goes through escape_path)
- Shipped in: not yet released

`new_request` keeps Go's RawPath (url_parts' seventh element, "" when the
default escaping of the path gives the text as written), and the client
sends `escaped_path()`: the path as written while it is a valid encoding
of the decoded one, else the decoded path escaped. Request.escaped_path
on a server was already so.

The in memory network of a test (and the Go backend's fakeNet) looked a
listener up by host and the URL's explicit port only, so
`os.net.listen("host:80")` was not reached by `http://host/`; it now
uses 80 (443 for https) when the URL has no port, as a real dial does.
The real client already did.

## 2026-11-04: fix: full duplex over HTTP/2 on TLS (native client)

- Kind: fix
- Commit: this push
- Found by: the owner's v0.36.0 evaluation (b24): an HTTP/2 over TLS request whose body is an http.pipe got its head, but the server saw no more of the body until it ended, so a full duplex exchange (decision 0433) timed out on its first read; HTTP/1.1, h2c, the Go backend and Go's client were fine
- Cause: the h2 pump polls a duplex body between reads with a 10 ms deadline (and the PING keepalive uses one too) set by hclient.put_deadline, which set only the socket's deadline; a TLS connection's reads wait on tlsio's own, so the read waited for the request's deadline instead
- Go reference: net/http h2_bundle.go clientStream.writeRequestBody (the body is written as it is read, independent of the response's reads)
- Test: golden http_duplex_h2tls (arm64 and LLVM): three ping-pong exchanges over HTTP/2 by ALPN on TLS, the second on the same connection, the third with a PING keepalive configured; origin/main's std timed out after the first echo on both
- Timing: none (a missing wakeup, no hot path changed)
- Shipped in: not yet released

## 2026-11-03: fix: the client pools a connection after a request whose body came from a source

- Kind: fix
- Commit: this push
- Found by: Xo Lang 1 (2026-11-03): a streamed request with a body source ended through hclient.end_shared, which always closed the connection
- Go reference: net/http transport.go persistConn.readLoop (a kept alive response read to its end and `pc.wroteRequest()`)
- Test: golden http_client_pool_body (arm64, LLVM, Go parity): six POSTs with body sources (Client.open chunked and with a Content-Length, Client.send) and two GETs on one connection, where the native client dialed four
- Timing: bench/results/http-client-pool-body-2026-11-03.md: 2000 keep-alive POSTs with body sources through Client.open, 0.199 of the wall time (0.194 to 0.225) and 0.124 of the CPU time on linux/arm64; Client.send within noise (it already pooled)
- Shipped in: not yet released

A request whose body came from a source (Request.with_body_reader) sent
through Client.open went full duplex (decision 0433) and its connection
was closed at the response's end, since the sender task might still be
writing. The sender now marks the body sent in full after its last
write, and the connection goes back to the pool when the response kept
it alive, was read to its end, and the body was marked sent, as Go's
Transport does. Since a server answers once the body's last byte has
arrived, the reader may finish first; it waits up to 50 ms for the mark
(net/http's maxWriteWaitBeforeConnReuse) and closes the connection if
the body is still not sent.

## 2026-11-03: fix: the proxy's tunnel keeps the target connection until its client-to-target copy ends

- Kind: fix
- Commit: this commit
- Found by: the Lead's review of the entry below (it avoided the path instead of fixing the lifetime)
- Go reference: net/http/httputil reverseproxy.go handleUpgradeResponse
- Test: golden http_reverse_proxy_ws (now an upgrade without a body source) as a wasip2 command and on arm64, LLVM and Go parity
- Timing: none

tunnel marks its client stream shared, so when the target side ends the
stream closes only the connection's descriptor and the connection is
freed with the client-to-target task's reference. A proxied upgrade
goes without a body source again, like any request without a body.

## 2026-11-03: fix: http.reverse_proxy no longer writes to a freed connection after a proxied upgrade ends

- Kind: fix
- Commit: this commit
- Found by: the Lead's wasip2 command run: http_reverse_proxy_ws trapped 5 of 5 ("unknown handle index" in wasi_sock_write) after the previous entry's fix
- Go reference: net/http/httputil reverseproxy.go handleUpgradeResponse (the backend connection lives until both copies end)
- Test: golden http_reverse_proxy_ws as a wasip2 command (internal/selfhost TestWasmCommandGoldens), on arm64, LLVM and Go parity
- Timing: none

The previous fix sent a request without a body source when it had no
body, which included a WebSocket upgrade. Without a source the client
made no full duplex exchange, so when the tunnel's target side ended,
the connection was freed while the tunnel's client-to-target task still
wrote on it. An upgrade keeps its source again.

## 2026-11-03: fix: http.reverse_proxy sends a request without a body without one, so a GET is retried on a fresh connection

- Kind: fix
- Commit: this commit
- Found by: the http_reverse_proxy_ws golden's flake (Lead's queue, 2026-11-03): under the full arm64 golden load 1 to 3 runs in 200 failed ("dial tcp ...: connection refused", `protocol error: Get ".../bye": EOF`, or a bad handshake with no proxy log)
- Go reference: net/http/httputil reverseproxy.go (`if req.ContentLength == 0 { outreq.Body = nil }`), net/http transport.go shouldRetryRequest and Request.isReplayable
- Test: golden http_reverse_proxy_retry (arm64, LLVM, Go parity): a target that closes a kept connection on its next request is retried on a new one, a POST's body is still forwarded, a bodiless request goes without a body; http_reverse_proxy_ws 50 and 100 times and an instrumented copy 800 times under the full arm64 golden load on the second linux/arm64 host, no failure
- Timing: none

The proxy handed every incoming request's body reader to its client,
so even a GET without a body went to the target as `Transfer-Encoding:
chunked` with an empty chunk, and, having a body source, was not
replayable: when the pooled connection it took had been closed by the
target, the client did not send it again on a new connection (decision
0303's rule, which Go's Transport has) and the proxy answered 502. A
request the server read no body for (HTTP/1.1 Content-Length 0 or
neither it nor chunked, net/http's ContentLength 0) now goes on with no
body, as httputil sets `outreq.Body = nil`; the Go backend's proxy
(std/rt http_proxy.go) does the same with net/http's NoBody. Requests
with a body are unchanged (not replayable, as in Go).

## 2026-11-03: feature: http.Client reuses connections in a wasip2/wasm command

- Kind: feature
- Commit: this commit
- Found by: decision 0447 R2g
- Go reference: none
- Test: golden http_client_keepalive as a wasip2/wasm command (TestWasmCommandGoldens)
- Timing: none

http.Client in a wasip2/wasm command keeps idle HTTP/1.1 and HTTP/2
connections and reuses them, as natively (up to 100 idle, 2 per host for
HTTP/1.1, 90 s), instead of dialing (and, for https, a TLS handshake) for
every request.

## 2026-11-03: feature: HTTP/2 server in a wasip2/wasm command

- Kind: feature
- Commit: this commit
- Found by: decision 0447 R2f
- Go reference: none
- Test: goldens http_h2c, http_h2c_sniff, http_trailers, http_duplex, http_h2_config, http_conn_fault as wasip2/wasm commands (TestWasmCommandGoldens)
- Timing: none

http.Server serves HTTP/2 in a wasip2/wasm command: h2c (with_protocols
UnencryptedHttp2) and over TLS when ALPN chooses h2, with streamed
bodies and trailers. http.Client in a command speaks HTTP/2 too (ALPN h2
over https, prior knowledge with UnencryptedHttp2), one connection per
request (no connection pool on wasm).

## 2026-11-03: feature: https in a wasip2/wasm command

- Kind: feature
- Commit: this commit
- Found by: decision 0447 R2d, R2e
- Go reference: none
- Test: goldens http_tls, http_tls_keys, http_tls_state, tls_client, tls_coalesced_finish, tls_deadline, tls_mtls, tls_peer_certs, websocket_tls, websocket_tls_deadline as wasip2/wasm commands (TestWasmCommandGoldens)
- Timing: none

http.Server.with_tls and with_client_auth serve TLS in a wasip2/wasm
command, and http.Client does https there (with_roots and with_cert
included), over wasi:sockets with the native TLS code; HTTP/1.1 only (no
HTTP/2 on wasm yet). There are no system roots on wasm: pass roots with
with_roots. A wasi:http component still refuses with_roots and with_cert
(its host does TLS).

## 2026-11-03: feature: full duplex http.Client requests in a wasip2/wasm command

- Kind: feature
- Commit: this commit
- Found by: decision 0447 R2c
- Go reference: none
- Test: goldens http_pipe, http_reverse_proxy, http_reverse_proxy_ws
- Timing: none

A streamed request body in a wasip2/wasm command is sent from a task while
the response is read, as natively (it was sent whole first): a wasm server
record now has a detached task scope (rt.wasi_scope_new), which holds the
sender, and closing the record cancels and waits for it. A request's
remote_addr is now the peer's address in a command too (it was empty):
wasi:sockets' tcp-socket.remote-address (rt.wasi_sock_peer).

## 2026-11-03: feature: http.Client in a wasip2/wasm command

- Kind: feature
- Commit: this commit
- Found by: decision 0447 R2b
- Go reference: none
- Test: goldens http_client, http_client_body_reader, http_client_policy, http_client_stream, http_fault, http_form, http_gzip, http_gzip_responses, http_server, http_server_request, multipart_basic, http_client_responses, http_stream, websocket, websocket_deadline on wasip2/wasm commands (TestWasmCommandGoldens) and natively
- Timing: none

A wasip2/wasm command's http.Client is the native client over wasi:sockets
(hclient's wasm arms), streamed request and response bodies included; a
wasi:http component's client is unchanged. https and HTTP/2 return an error
saying they are not on wasm yet, each request dials (no pool), and a streamed
request body is sent before the response is read (no full duplex yet).

## 2026-11-03: feature: http.Server in a wasip2/wasm command

- Kind: feature
- Commit: this commit
- Found by: decision 0447 R2a (lead-approved 2026-11-03)
- Go reference: none
- Test: goldens http_wire, http_multipart_stream, http_stream_body on wasip2/wasm commands (TestWasmCommandGoldens) and natively
- Timing: none

A wasip2/wasm command (a program that does not call http.serve_component) can
serve HTTP/1.1 with http.Server over wasi:sockets: the native server's code runs
unchanged (parsing, headers, response writing, error texts), and only its
connection I/O and runtime records have wasm arms (decision 0447 "http.Server in
a command"). Not yet on wasm: HTTP/2, TLS, the peer address (remote_ip is
empty), idle connection parking, half-close.

## 2026-11-03: perf: parse_set_cookie and Expires dates no longer lower case copies

- Kind: perf
- Commit: this commit
- Found by: bench/perf/cookie setcookie 1.52 of Go's wall; profile: cookie_date 36%, unicode.case_append 14%
- Go reference: net/http cookie.go (go1.26.1)
- Test: bench/perf/cookie (twincheck), `xo test --std-module=native/http`, internal/stdtest http corpus

Attribute names, SameSite values, the weekday and the month compare with `equal_fold` against constant
names (the values are ASCII there, so the result is the lower cased compare's) instead of allocating a
lower cased copy each time; the weekday and month lists are consts. Xo / Go wall of setcookie: 1.52 to 1.02 (host 1, 15 rounds).

## 2026-11-02: fix: an HTTP/2 request's trailers were copied from freed memory

- Kind: fix
- Commit: this commit
- Found by: Lang 1's XO0915 lint draft (xo vet --cost, a raw address used after its value was released), after the 2026-11-02 race stress found the class in std
- Go reference: none
- Test: cmd/xo TestVetCostStd with XO0915 (the lint's own push; it reports these lines without the fix)
- Timing: none
- Shipped in: every release with HTTP/2 request trailers, through v0.35.0

A native HTTP/2 server joined each request trailer into a line and copied
it with `put(..., raw.data(line), line.len())`, the line's last use, so it
was released before the copy (gotchas/raw-address-outlives-bytes.md). The
handler saw the right trailers while nothing reused the block; the line
now stays alive past the copy, as does the Content-Length value parsed
the same way (shared with the header list, so only its count dropped).

## 2026-11-02: fix: a chunked request body was copied from freed memory

- Kind: fix
- Commit: this commit
- Found by: Lang 1's race stress (http_wire and http_client_body_reader built with --race on linux/arm64: a heap-use-after-free in http1.x_put)
- Go reference: none
- Test: internal/llvm TestRaceStdNoUseAfterFree/http_wire (a --race build reports the use after free before the fix)
- Timing: none
- Shipped in: v0.12.0 (bedead10b, http1.xo) through v0.35.0

A native HTTP/1.1 server decoded a chunked request body into a string and
appended it to the request's body buffer with
`x_put(..., raw.data(c.body), c.body.len())`; the string's last use was in
those arguments, so it was released before the copy
(gotchas/raw-address-outlives-bytes.md). The handler saw the right body
as long as nothing reused the block in between; a --race build reported
it on every chunked request. The string now stays alive past the copy.

## 2026-11-02: fix: an HTTP/2 response's content-length was copied from freed memory

- Kind: fix
- Commit: this commit
- Found by: Lang 1's race stress (every golden that serves HTTP/2 built with --race on linux/arm64: a heap-use-after-free in h2w.field, http_h2c_sniff, http_h2c, http_conn_fault, the tls goldens)
- Go reference: none
- Test: internal/llvm TestRaceStdNoUseAfterFree/http_h2c_sniff (a --race build reports the use after free before the fix)
- Timing: none
- Shipped in: v0.12.0 (b8cc0fe48, h2w.xo) through v0.35.0

A native HTTP/2 response whose handler set no Content-Length got one
from its body's length, formatted into a string whose last use was in
the arguments of the call that copied it (`field(..., raw.data(cl),
cl.len())`), so the string was released before the copy
(gotchas/raw-address-outlives-bytes.md). The copy read the right bytes
as long as nothing reused the block in between, which is why no output
showed it; a --race build, whose heap is the C library's, reported it on
every such response. The string now stays alive past the copy.

## 2026-11-02: perf: a native Response no longer carries the wasm stream's field (decision 0447)

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 3 (the /alloc drop at d71e1b442), Xo Lang 4 and Xo Lang 2 (measured at two data layouts)
- Go reference: none
- Test: native/http's tests, the http goldens (internal/arm64 TestGoldenRun), the wasm component tests (internal/selfhost TestWasmComponent*); bench/perf/jitter /alloc on the linux/arm64 host
- Timing: bench/perf/jitter /alloc on the first linux/arm64 host, one job (l4pts13, 3 rounds): req/s 146,450 against main's 131,181 at 32 connections (1.116) and 148,761 against 133,049 at 256 (1.118), CPU a request 0.891 and 0.894; the experiment that removed the field measured the same (143,480 and 149,392)

decision 0447 gave every Response a hidden `ws: rt.Any?` for a wasm
`http.stream`'s function; a native server paid for the word on every
response. The function now lives in the `stream` field, which on native
holds the stream's number (as an Any of Int, made only for a streamed
response).

## 2026-11-02: perf: a slow keep-alive connection stops lingering before it parks (decision 0449)

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 2's memory review of the 20 ms linger
- Go reference: none
- Test: Xo Lang 2's linger-mem runs (think time 10 and 30 ms at 2,000 and 10,000 connections, a 40,000 connection burst) and bench/perf/jitter /alloc on the linux/arm64 host
- Timing: 30 ms think time at 2,000 connections: RssAnon 70 to 71 MB to 56 to 63 MB, req/s 64k to 52 to 56k; busy connections unchanged (178k req/s); /alloc req/s 1.072 at 32 connections and 1.037 at 256 against main in the same job, CPU a request 0.965 and 0.961 (bench/perf/jitter, first linux/arm64 host, 3 rounds, 2026-11-02, both on main with a1 P, 25d23d18e, since reverted)

A connection whose 20 ms linger timed out parks at once on its later
idles, until it comes back within 20 ms of its last park, so slow
keep-alive traffic no longer holds a stack for 20 ms of every gap.

## 2026-11-02: fix: an HTTP/2 request whose trailers arrive late no longer hangs its streaming handler

- Kind: fix
- Commit: this commit
- Found by: Xo Lang 1, http_trailers under load (1 run in about 1,900, 2026-11-02)
- Go reference: net/http's http2 server (processTrailerHeaders ends the body and wakes its reader)
- Test: internal/llvm TestH2Diff's raw "late trailers" case (HEADERS and DATA, a 300 ms pause, then the trailers): the native server never answered before this fix
- Timing: none

A trailing HEADERS frame that ended a request's stream marked the body
done but did not wake a `handle_stream` handler already waiting for more
of it (a DATA frame's END_STREAM did), so a request whose trailers came
in a later read than its last DATA hung until the client gave up. The
code is the same in v0.35.0, where a client sending HTTP/2 request
trailers (a gRPC client, for one) to a `handle_stream` route could hit
it.

## 2026-11-02: perf: an idle HTTP/1.1 connection parks only after 20 ms without a request (decision 0449)

- Kind: perf
- Commit: this commit
- Found by: xo-lang-4, the /alloc regression bisect for 0.36 (Lead, 2026-11-02)
- Go reference: none (net/http keeps a goroutine per connection and never parks)
- Test: bench/perf/jitter /alloc on the linux/arm64 host (alloc-points, exclusive, 3 rounds) and the idle connection tests (bench/perf/idleconn at 2,000 and 40,000 connections)
- Timing: /alloc req/s against v0.35.0, c32 0.775 to 1.023, c256 0.651 to 1.027; CPU a request 1.327 to 0.972 and 1.536 to 0.973 (linux/arm64, 2026-11-02)

A plain HTTP/1.1 keep-alive connection decided to park from the gap since
its last response (2 ms). Under load on Linux, readiness reaches a
connection one poll later, so busy connections looked idle and parked on
most requests (57% at 32 connections, 91% at 256), each park and resume
costing a closure, the server's parked list and a task. Now the
connection, after its peek finds no bytes, waits on its own stack with a
20 ms timer and parks only when nothing came. Idle connections still
park, 20 ms later. TLS and HTTP/2 connections are unchanged.

## 2026-11-02: feature: handle_stream and trailers in a wasi:http component (decision 0447)

- Kind: feature
- Commit: this commit
- Found by: xo-lang-4, decision 0447's next stage (owner, 2026-11-02)
- Go reference: net/http's Request.Body and Request.Trailer, and the TrailerPrefix response header
- Test: internal/selfhost TestWasmComponentHTTP (the stub host: a body in pieces, a declared and an undeclared trailer, a streamed response in two writes with a trailer and a forbidden one) and TestWasmComponentServe (wasmtime serve: 100,000 bytes in several reads, a body in three pieces 50 ms apart read as they arrive, a request trailer, a response trailer, a 1 MiB streamed response, a route that answers after one read)
- Timing: none (the host holds the connection)

On `--targets=wasip2/wasm`, a `handle_stream` route's `BodyReader` reads the
request body from the host's input-stream as it arrives, in place of the whole
body read first, and `BodyReader.trailers()` has the trailers the request's
`Trailer` field declared (`incoming-body.finish`), as the native server keeps
them. A route that answers without reading to the end leaves the rest to the
runtime. An `http.stream` response goes to the host as it is written: its
head with the first write or flush (a missing Content-Type detected from
those bytes, as net/http does), its body through the output-stream, then
`BodyWriter.set_trailer`'s trailers through `outgoing-body.finish`, names a
trailer may not carry dropped (a host may send only declared trailers, and only to a
client that sent `TE: trailers`, as wasmtime serve does). In process (`Router.call`) the stream is
collected as before, its trailers as Go TrailerPrefix headers. A client's
request trailers still fail.

## 2026-11-02: feature: http.Client in a wasi:http component (decision 0447)

- Kind: feature
- Commit: this commit
- Found by: xo-lang-4, decision 0447's next stage (owner, 2026-11-01)
- Go reference: none (Go has no wasi:http client in net/http)
- Test: internal/selfhost TestWasmComponentClient (wasmtime serve against a Go server)
- Timing: none (the host holds the connection; no native comparison)

On `--targets=wasip2/wasm`, `Client.send` and `Client.open` go through
`wasi:http/outgoing-handler`, with the same redirects, Net.only checks,
Authorization stripping and gzip as native. Limits of this stage: a request
blocks the instance while it runs; request and response are held whole, so `open`
reads its response before it returns and a piped body is read before it is sent;
each timeout is the host's per-phase one, not one deadline across redirects;
request trailers fail; `with_roots` and a client with `with_cert` fail, since the
host does TLS. A component that does not reach the client imports nothing new.

## 2026-11-01: fix: an HTTP/2 connection whose idle park was refused re-reads before it waits (decision 0449 stage 1c)

- Kind: fix
- Commit: this commit
- Found by: TestH2Diff (a raw case answered nothing for 30 s under load, native "open" where Go answers)
- Go reference: none (net/http's http2 server blocks in a goroutine and has no park)
- Test: internal/llvm TestH2Diff, 3 rounds of 4 concurrent runs on the Mac: 12 of 12 clean (about 39 to 41 s each), where the default failed about 1 in 4 and ran 79 s once; XO_THREADS=1 and parking off were clean
- Timing: none

hconn.park_ready clears the socket's read readiness and peeks; bytes the peek found refuse the park, but no edge announces them any more, and the h2 loop waited at once, so the connection sat silent until a deadline. The loop now reads once more after a refused park.

## 2026-11-01: feature: http.serve_component builds a whole wasi:http component, with no other tools (decision 0447)

- Kind: feature
- Commit: this commit
- Found by: xo-lang-4, decision 0447, the stage the owner accepted 2026-11-01
- Go reference: none (Go's wasip2 handlers come from componentize-go or TinyGo with ydnar/wasi-http-go)
- Test: internal/selfhost TestWasmComponentGoldens (18 native goldens answered the same under `wasmtime serve`), TestWasmComponentTargets, TestWasmComponentServe (testdata/wasm_http_serve under `wasmtime serve`, after `wasm-tools validate`; skips where they are not installed), TestWasmComponentHTTP (the core module under a stub host in Node)
- Timing: hello program under `wasmtime serve`, linux/arm64, 32 connections, 3 rounds, median: Xo 83,339 req/s, 281,733 bytes; Rust 89,794 req/s, 79,127 bytes; TinyGo 29,114 req/s, 2,910,541 bytes (ADR 0447)

`xo build --targets=wasip2/wasm` of a program calling `http.serve_component`
writes the component itself (XO1514 on wasip1/wasm, XO1515 for files, processes
or sockets, XO1516 with no router): wasm-tools and the preview 1 adapter are gone
from the path, and `wasmtime serve` takes the output as it is, with only the
wasi:http/proxy world. The runtime's system calls use WASI 0.2 in place of
preview 1: stdout, stderr, clocks and random work; arguments and the
environment are empty; a read of stdin faults. A request now reaches the router
as the native server's does (escaped path, repeated fields, Host), the response
keeps repeated fields and sniffs a missing Content-Type as the server does, and
`http.stream` works, going out whole. A route that faults ends the
request with an error from the host and the next request gets a new instance.

## 2026-11-01: fix: a parked idle connection answers its next request on Windows (decision 0449)

- Kind: fix
- Commit: this commit
- Found by: the Lead's tier, windows/amd64 sample at 28a0dbcf2 (http_server, http_tls)
- Go reference: none (the runtime's own poller)
- Test: goldens http_server and http_tls on windows/amd64 (`OFFLOAD_ENV=offload-win.env amd64-run.sh REV NAME http_server http_tls`)

On Windows the poller watches only descriptors someone waits on, and a
connection parked between requests counted as nobody, so its next request
was never seen and the client timed out.
A parked connection is now watched for reads, with the AFD poll and with
WSAPoll. Other systems are not affected: their poller registers every
descriptor when it opens.
## 2026-11-01: fix: no continuation record outlives a stopped server (decision 0449)

- Kind: fix
- Commit: this commit
- Found by: the Lead's tier, TestGoldenTasks/http_conn_fault (1 live at exit)
- Go reference: none (parking is Xo's own)
- Test: golden http_conn_fault, repeated: 0 of 600 runs leak in its two cut down forms that leaked 8 to 9 and 5 of 200 before

An HTTP/2 connection that fired, or parked, after the stopping server's
resumer had taken its last look left its 64 byte record on the server's lists
with no one to free it, so XO_LEAKCHECK sometimes reported 1 live block at
exit. The server now frees what is left when it is freed.

## 2026-11-01: feature: http.serve_component serves a wasi:http component's request, wasm (decision 0447)

- Kind: feature
- Commit: this commit
- Found by: xo-lang-4, decision 0447 step 1
- Go reference: none (Go's wasip2 handlers come from componentize-go or TinyGo with ydnar/wasi-http-go)
- Test: internal/selfhost TestWasmComponentHTTP (testdata/wasm_http: a stub wasi:http host under Node, an instance per request and one for every request)
- Timing: the hello handler under wasmtime serve, linux/arm64 offload host, one 20 s loadgen run at 32 connections: Xo 75,620 req/s, Rust 85,410, TinyGo 28,109 (Go has no row: standard Go needs componentize-go); decision 0447 Step 1 results

A program whose main builds a router and calls `http.serve_component(r)`
builds, on the wasm backend, as the core module of a `wasi:http/proxy`
0.2 component: it exports `wasi:http/incoming-handler@0.2.0#handle` and
`cabi_realloc` in place of `_start`, runs main for each request, and
answers through `Router.call`. The request's method, path with query,
fields (keyed by Go's canonical name, a repeated name's first value) and
body reach the router; the response's status, fields and body go back.
Step 1 keeps the preview 1 imports of the rest of the runtime, so the
host wraps the module with the preview 1 adapter (wasm-tools component
embed and new). Off wasm, serve_component faults.

## 2026-11-01: perf: idle HTTP/2 connections with a ping interval or deadline park (decision 0449 stage 1c)

- Kind: perf
- Commit: this commit
- Found by: decision 0449 (idle connection memory)
- Go reference: net/http's http2 server keeps a goroutine per idle connection; this has no Go counterpart
- Test: internal/llvm/testdata/golden/http_h2_config (server pings on a parked connection), http_h2c, http_h2c_sniff, http_stream_body_h2
- Timing: none; the 40k idle h2 check is internal/stdtest TestIdleConnections40kH2

An idle h2 connection whose H2Config has send_ping_timeout, ping_timeout or a deadline used to keep its task. It now parks with the timer, and the timer resumes the open connection to send the ping or end it.

## 2026-10-31: feature: http.Status and the status code constants (decision 0269 stage A)

- Kind: feature
- Commit: this commit
- Found by: decision 0269 (open sets as newtypes with named constants)
- Go reference: net/http's StatusOK ... StatusNetworkAuthenticationRequired (62 codes)
- Test: internal/llvm/testdata/golden/http_status_consts (arm64, LLVM, Go parity): the constants' values, constant patterns, a range arm, equality
- Timing: none (compile time constants)

`http.Status` is `type Status struct(Int)` with a constant per code Go names. It is additive:
no signature changes yet (`status: Int` stays), so no existing program changes.

## 2026-10-31: perf: idle HTTP/2 connections park (decision 0449 stage 1c)

- Kind: perf
- Commit: this commit
- Found by: decision 0449 (an idle h2 connection held a fiber stack, 32 KB resident, two mappings)
- Go reference: Go 1.28 parks idle HTTP/2 connections
- Test: internal/stdtest TestIdleConnections40kH2 (XO_TEST_IDLE40K=1, linux)
- Timing: linux/arm64 host, 40,000 idle h2 connections over TLS, each with a second request answered: 470 server mappings, RssAnon 918,892 kB (about 23 KB each); before, an idle h2 connection held a fiber stack (two mappings, 32 KB resident) so a server stopped near 32,000

An HTTP/2 connection with no open stream, nothing buffered or to send, and no
ping interval or deadline parks while it waits for its next frame: its task
ends and a resumer restarts the frame loop on bytes. Connections with a ping
interval (Server config) do not park yet.

## 2026-10-31: perf: a small TLS read no longer pins its 16 KiB buffer (decision 0449)

- Kind: perf
- Commit: this commit
- Found by: the TLS memory survey of decision 0449 (50 KB an idle TLS connection against 6 KB plain)
- Go reference: crypto/tls reads into a pooled buffer and copies out records
- Test: internal/stdtest TestIdleConnections40kTLS (XO_TEST_IDLE40K=1, linux)
- Timing: linux/arm64, 40,000 idle TLS connections, server RssAnon 2,029,152 kB to 752,584 kB (about 18.8 KB each), 362 mappings

A TLS read of at most 4,096 bytes is copied to a block of its own size, so
the ClientHello's views (sni, alpn, record bytes) no longer keep the 16 KiB
read buffer alive for the life of the connection.

## 2026-10-31: perf: idle keep-alive connections park, on plain and TLS HTTP/1.1 (decision 0449 stage 1)

- Kind: perf
- Commit: this commit
- Found by: the owner, accepting decision 0449 (an idle connection held a fiber stack, two mappings, so a server stopped near 32,000 idle connections at the default vm.max_map_count)
- Go reference: net/http holds an idle connection in a goroutine whose stack is heap; Go 1.28 parks idle HTTP/2 connections
- Test: internal/stdtest TestIdleConnections40k and TestIdleConnections40kTLS (XO_TEST_IDLE40K=1, linux: 40,000 idle connections, every one answers a second request, under 20,000 mappings: 327 and 626)
- Timing: bench/perf/http CRUD server, 32 busy connections, linux/arm64, 5 runs: 189,308 to 188,129 req/s (-0.6%, within noise), CPU 11.68 to 11.76 us per request; idle h1 RssAnon 6.4 KB a connection (plain), 50 KB (TLS)

A connection that waits for its next request with nothing buffered no
longer holds a task and its stack: the server parks it in the poller and
starts a task when bytes arrive (a stack from the cache), or closes it at
the idle timeout. A connection whose last gap between requests was under
2 ms keeps its fiber, so a busy keep-alive client pays nothing. TLS
connections park once the engine holds no plaintext and no partial record.
HTTP/2 connections do not park yet.

## 2026-10-30: fix: Router.call builds for wasm (decision 0447)

- Kind: fix
- Commit: this commit
- Found by: xo-lang-4, starting decision 0447's wasi:http component (a router program's wasm build failed)
- Go reference: none (Go's ServeMux has no in-process call; net/http/httptest's recorder is the nearest)
- Test: internal/selfhost TestSelfhostWasm http_router_paths, http_router_hooks, http_handler_fault (off wasm_unsupported.txt)
- Timing: none

A program that dispatched through `Router.call` failed its wasm build
("the wasm backend does not support the call `rt.heap_big`"): the
collection of a streamed body reached the native server's buffer sink,
which allocates with llrt's heap. No stream is made on wasm, so the
collection is skipped there by a test the build decides, and the sink
is not built.

## 2026-10-30: behavior: http.pipe() holds at most 64 KiB before its reader's first read, every backend (decision 0444)

- Kind: behavior
- Commit: this commit
- Found by: the owner, accepting decision 0444
- Go reference: io.Pipe holds nothing (every Write waits for a Read); the cap bounds Xo's pre-read window
- Test: internal/llvm/testdata/golden/http_pipe ("cap": 8 writes of 32 KiB from a task wait before open, then all 256 KiB arrive; arm64, LLVM, Go parity)
- Timing: none

Writes before a pipe's reader starts are held up to 64 KiB; a write that
would pass that waits for the reader, as every write does once the
reader has started. A writer task running ahead of a late `open` no
longer grows memory without bound.

## 2026-10-30: fix: a full duplex client request no longer frees its connection under the body's sender, native (decision 0433)

- Kind: fix
- Commit: this commit
- Found by: the lead's tier 302 on the linux/arm64 offload host (ThreadSanitizer: "data race in x.hclient.end_keep", the previous read in x.hclient.send; http_reverse_proxy, tls_mtls)
- Go reference: net/http's persistConn closes the connection, and a writeLoop still writing gets an error; the connection's memory lives on with its last user (Go's collector)
- Test: TestGoldenTasks (ThreadSanitizer) http_reverse_proxy, tls_mtls, http_pipe, http_duplex; goldens on arm64 and LLVM
- Timing: none

A request streamed from a task (`Client.open` with a BodyReader body)
ended its connection when the response ended or failed, freeing its TLS
state and clearing the connection's record while the sending task could
still be writing on them. Now that end only closes the descriptor (the
sender's write fails) and records the failure; the rest is freed with the
exchange's last reference, after the sender is gone.

## 2026-10-30: fix: a client done with a request closes its piped body, every backend (decisions 0433, 0444)

- Kind: fix
- Commit: this commit
- Found by: Xo lang 5, while gating decision 0444 (a pipe writer that never closes, to a server that drains the body before it answers)
- Go reference: net/http's Transport closes Request.Body when it is done with the request; io.PipeReader.Close makes the writer's Write return io.ErrClosedPipe
- Test: internal/llvm/testdata/golden/http_pipe ("unclosed": open times out after 1 s, the next write fails "io: read/write on closed pipe"; arm64, LLVM, Go parity)
- Timing: none

A request whose body is an `http.pipe()` reader, where the writer never
closes it, hung when the client gave up (a timeout, an error): the Go
backend's transport waited for its body copy, which read the pipe
through a body it could not close, and the native client's exit waited
for its sending task, blocked on the pipe. The client now closes the
pipe's reading end when it is done with the request (an error, the
response's end, `ClientStream.close`, after `send`), so a waiting write
and every later one fail with `io: read/write on closed pipe`. The server
side is unchanged and matches net/http: after a handler leaves a body
unread it drains up to 256 KiB, waiting for a body that never ends.

## 2026-10-30: fix: a pipe write before the first read is held, every backend (decision 0444)

- Kind: fix
- Commit: this commit
- Found by: Xo lang 5's gRPC client (agent survey 2026-10-30, item 3: write the first message, then Client.open, hung)
- Go reference: io.Pipe (each Write waits for a Read); Xo departs from it before the first read
- Test: internal/llvm/testdata/golden/http_pipe ("prewrite": two writes and a close in the task that then opens the request; arm64, LLVM and the Go backend)
- Timing: none (the held pieces are the bytes the writer gave)

`PipeWriter.write` before the BodyReader's first read now holds the
piece and returns; the reader takes the held pieces first, then waits
on the writer as before. The Go backend waited forever on the first
such write and the native backends on the second.
Also native: a `Client.open` stream with a piped body, read to its end
without `close()`, kept its sending group live until exit (one
allocation under XO_LEAKCHECK); it now closes when the body ends or
fails, as Go's transport is done with the request then.

## 2026-10-30: fix: an undeclared trailer after a short streamed body, Go backend (decision 0433)

- Kind: fix
- Commit: this commit
- Found by: Xo lang 5's header and trailer probe across backends (agent survey 2026-10-30, item 3)
- Go reference: net/http's chunkWriter.writeHeader (a handler done within the buffer gets a Content-Length from the headers seen at WriteHeader)
- Test: internal/llvm/testdata/golden/http_trailers ("h1 short": `write("hi")` then `set_trailer("Grpc-Status", "0")`; the Go backend's output equal to the native one's)
- Timing: one flush when an HTTP/1.1 streamed response sets a trailer

On HTTP/1.1 the Go backend's server wrote a streamed response's status
before running its function, so net/http fixed the framing from the
headers it had then: a body short enough to end with the handler went
out with a Content-Length, and a trailer set without
`with_trailer_names` was dropped. The native server sends such a
response with a body and a trailer chunked and delivered the trailer.
The Go backend now flushes the head when a trailer is set after a body
byte (or after the next write, for one set first), so it goes out
chunked too and its trailers follow the last chunk; a streamed
response without trailers keeps its Content-Length, and one with no
body keeps Content-Length: 0 and drops undeclared trailers, as before.
Header and trailer names were already canonical on every backend, in
both directions and over a raw peer's lowercase names.

## 2026-10-30: feature: Client H2Config PING keepalive, native (decision 0434)

- Kind: feature
- Commit: this commit
- Found by: the 0434 open list (the client ignored send_ping_timeout and ping_timeout)
- Go reference: net/http's Transport.HTTP2.SendPingTimeout and PingTimeout ("http2: client connection lost")
- Test: internal/llvm/testdata/golden/http_h2_config ("client ping": a raw server never answers; it sees the client's PING and the request fails; native output equal to the Go backend's)
- Timing: none unless set (the read keeps its deadline)

While a request waits for the server's next frame, a read idle for
send_ping_timeout sends a PING, and a further silence of ping_timeout (0:
15 s) fails the request with "http2: client connection lost" and drops
the connection. Go pings from a reader of the connection even with no
request; the native client has no such task, so an idle pooled
connection is not probed.

## 2026-10-30: feature: Client H2Config.max_read_frame_size, native (decision 0434)

- Kind: feature
- Commit: this commit
- Found by: the 0434 open list (the native client always sent MAX_FRAME_SIZE=16384)
- Go reference: net/http's Transport.HTTP2.MaxReadFrameSize (the SETTINGS_MAX_FRAME_SIZE the transport sends and the frame size it accepts)
- Test: internal/llvm/testdata/golden/http_h2_config ("client tuned" now reads MAX_FRAME_SIZE=65536; native output equal to the Go backend's)
- Timing: none (one setting per connection)

The native client sends the configured size (16384 to 16777215, 0 the
default 16384) in its SETTINGS and accepts frames up to it; a larger
one is FRAME_SIZE_ERROR as before. A different size is a different
pooled connection.

## 2026-10-30: fix: a 431 or 413 reply survives unread request bytes on Linux, native server

- Kind: fix
- Commit: this commit
- Found by: TestGoldenRun/http_server_limits on the linux/arm64 host ("big header: error" where macOS got the 431)
- Go reference: net/http's closeWriteAndWait (CloseWrite, then a wait before Close)
- Test: internal/llvm/testdata/golden/http_server_limits (arm64, LLVM, Go parity), on the linux/arm64 host
- Timing: none (the lingering read is on the error path only)

The server answered 431 (head over with_max_header_bytes) and 413 (body
over with_max_body), shut down its sending side and closed with the
rest of the request still unread; on Linux that close sends a RST, which
can discard the reply from the client's receive buffer, so a client
still writing a 100 KB header saw a connection error. The server now
reads and discards what the peer still sends (at most 500 ms or 1 MiB,
until the peer closes) before it closes, as net/http does.

## 2026-10-30: feature: H2Config.max_encoder_header_table_size, native server (decision 0434)

- Kind: feature
- Commit: this commit
- Found by: the 0434 open list (the native server ignored the field)
- Go reference: net/http's http2.Server.MaxEncoderHeaderTableSize
- Test: http_h2_config still equal to want.txt (no probe: the cap only shows as a dynamic table size update in a response's first header block)
- Timing: none

The HPACK encoder of a connection keeps its dynamic table within the
setting (0 or over 4096: 4096) and the peer's SETTINGS_HEADER_TABLE_SIZE,
whichever is smaller; a cap below 4096 is announced with a table size
update in the first block, as Go.

## 2026-10-30: feature: H2Config.write_byte_timeout, native server (decision 0434)

- Kind: feature
- Commit: this commit
- Found by: the 0434 open list (the native server ignored the field)
- Go reference: net/http's http2.Server.WriteByteTimeout
- Test: http_h2_config still equal to want.txt; no stalled-reader probe (the kernel's socket buffers make when a write blocks depend on the host, so a golden would not be stable)
- Timing: none (the default writes as before)

With the timeout set, the HTTP/2 server writes its pending output in
16 KiB pieces that each must finish within it, and closes the connection
when one does not (no write progress). Unset: one write, no deadline.

## 2026-10-30: feature: Client.with_h2_config, native (decision 0434)

- Kind: feature
- Commit: this commit
- Found by: the 0434 open list (the client stored its H2Config and ignored it)
- Go reference: net/http's Transport.HTTP2 (HTTP2Config): MaxReceiveBufferPerStream, MaxReceiveBufferPerConnection, MaxDecoderHeaderTableSize; the SETTINGS and WINDOW_UPDATE net/http's HTTP/2 transport opens a connection with
- Test: internal/llvm/testdata/golden/http_h2_config (a raw server reads a client's first frames; native output equal to the Go backend's)
- Timing: none (settings written once per connection)

The native client sends its H2Config in the connection's SETTINGS
(INITIAL_WINDOW_SIZE, HEADER_TABLE_SIZE when not 4096) and connection
WINDOW_UPDATE, sizes its HPACK decoder and flow windows by it, and keeps
clients with different configs on different pooled connections. The
default connection now opens as net/http does: MAX_FRAME_SIZE=16384 in
the SETTINGS, and a WINDOW_UPDATE of the whole 1 GiB (the window was
the same 1 GiB less 65535 before: now 65535 above it, as Go). Not applied
by the native client: max_read_frame_size (its frames stay 16 KiB),
the PING keepalive and write_byte_timeout. The golden also pins max_concurrent_streams: a second stream while
one is open gets RST_STREAM REFUSED_STREAM, as Go.

## 2026-10-30: feature: HTTP/2 client full duplex, native (decision 0433)

- Kind: feature
- Commit: this commit
- Found by: xo-lang-5's gRPC interop program (ping_pong deadlocked: "every task is blocked")
- Go reference: net/http's HTTP/2 transport writes the request body from its own goroutine while the response is read
- Test: internal/llvm/testdata/golden/http_duplex (an h2c row next to the h1 one); the gRPC interop client against the Xo server, 9 of 9 cases
- Timing: none on a request without a piped body

`Client.open` with a piped body (`http.pipe`) over HTTP/2 returns at the
response head while the body is still written. The h2 pump pulled the
body source with a blocking read, which waited for a message that the
program writes only after a response arrives. A feeder task now pulls
the source into a queue, and the pump polls the queue and reads the
socket with a 10 ms deadline while it is empty (the request's own
deadline is put back).

## 2026-10-30: feature: h2c and HTTP/2 settings, native (decision 0434)

- Kind: feature
- Commit: this commit
- Found by: the gRPC readiness survey of 2026-10-30, gap 10
- Go reference: net/http's Protocols (UnencryptedHTTP2), HTTP2Config (MaxConcurrentStreams, MaxReadFrameSize, MaxReceiveBufferPerConnection/Stream, MaxDecoderHeaderTableSize, SendPingTimeout, PingTimeout)
- Test: internal/llvm/testdata/golden/http_h2c, http_h2c_sniff, http_h2_config (native output equal to the Go backend's)
- Timing: none on the request path (a 24 byte preface peek on a plain connection that allows h2c); `xo bench --ab` on the linux/arm64 host (OFF_EXCLUSIVE, 10 rounds, bench/perf/h2loop: 3000 h2 TLS requests over 200 connections, base c51c9f4f1 and the per-connection settings 15febcd33): wall 226.7 to 227.6 ms (1.004, interval 0.993 to 1.015), CPU 225.0 to 230.0 ms (1.022), max RSS 44.5 to 45.7 MB (1.027), all within noise

A plain server with `UnencryptedHttp2` in `with_protocols` sniffs the
HTTP/2 preface and serves h2c next to HTTP/1.1 (h2c only: other
connections close); a client with it speaks prior knowledge, and its
idle pooled connections are kept apart from HTTP/1.1 ones. ALPN follows
the protocol sets on both sides (an h2 only TLS server closes a
connection that did not negotiate h2). `with_h2_config` sets each
server connection's settings, and `send_ping_timeout` / `ping_timeout`
send a PING after that much read silence and close when no ack follows.
Requests on h2c report `tls() == false` and scheme `http`.

## 2026-10-30: feature: protocol sets and HTTP/2 settings (with_protocols, with_h2_config, h2c)

- Kind: feature
- Commit: this commit
- Found by: decision 0434 (gRPC behind a mesh sidecar speaks h2c with prior knowledge)
- Go reference: net/http Protocols, Server.Protocols, Transport.Protocols, HTTP2Config (go1.26.1)
- Test: golden http_h2c (an h1 + h2c server answering both clients on one port, an h2c only server, ALPN with one protocol removed, an empty set)

`Server.with_protocols` and `Client.with_protocols` take a set of
`Protocol` (`Http1`, `Http2`, `UnencryptedHttp2`), Go's
`http.Protocols`; `UnencryptedHttp2` is HTTP/2 with prior knowledge on a
plain connection. `with_h2_config(H2Config)` sets Go's `HTTP2Config`
(streams, frame size, windows, HPACK tables, PING keepalive, write
timeout). Go backend only for now: the native backends store the set
and config until decision 0434's native step lands.

## 2026-10-30: feature: mutual TLS (with_client_auth, with_cert, verified_chains)

- Kind: feature
- Commit: this commit
- Found by: decision 0433 (gRPC over TLS and service meshes need mTLS)
- Go reference: crypto/tls Config.ClientAuth, ClientCAs, Certificates; ConnectionState.VerifiedChains (go1.26.1)
- Test: golden tls_mtls (each of the five modes against no certificate, another CA's, one without the clientAuth usage, and a good one)

`Server.with_client_auth(mode, roots)` asks a TLS client for its
certificate in one of Go's five modes and verifies it against `roots`
(the system's when None); `Client.with_cert(cert)` presents one when a
server asks; `TlsState.verified_chains` has the chains that verified the
client's certificate. The native TLS 1.3 server asks and verifies as Go
does (CertificateRequest with the CAs' names, the client's Certificate
and CertificateVerify, the clientAuth usage, Go's alerts); a native
client presents its certificate only when a named CA issued it
(Client.with_cert keeps the pool apart with a copy of the roots, the
system's too; TLS 1.2 clients and servers do client certificates since
the std/tls entry of 2026-10-30). Also on the native backends, `with_tls` keeps
the server's earlier settings (read timeouts, header and body limits)
instead of resetting them.

## 2026-10-30: feature: full duplex and pipe (with_full_duplex, pipe, PipeWriter)

- Kind: feature
- Commit: this commit
- Found by: decision 0433 (gRPC bidirectional streams)
- Go reference: net/http ResponseController.EnableFullDuplex, io.Pipe and PipeWriter.CloseWithError (go1.26.1)
- Test: golden http_pipe (a piped request body read in pieces by the server, a body cut with an error, a local reader's max and error), the same output on the Go and native backends

`Response.with_full_duplex()` lets a streamed handler read the request
body while it writes over HTTP/1.1 (HTTP/2 streams always may), as
`EnableFullDuplex`. `http.pipe()` returns a PipeWriter and the
BodyReader it fills (Go's io.Pipe): each write waits for the reader,
`close` ends the body, `close_with_error(e)` makes the reader's next
read `Err(e)`, and a write after close is `Io("io: read/write on closed
pipe")`. With `Request.with_body_reader` and `Client.open` one task
writes a request while another reads the response. The native HTTP/1.1
full duplex wire part (server respond, client open with a body sender
task; golden http_duplex) is in; the native HTTP/2 client's non-blocking
pipe pump is still to come; `pipe` works on every backend.

## 2026-10-30: feature: trailers (decision 0433)

- Kind: feature
- Commit: this commit
- Found by: the gRPC readiness survey of 2026-10-30, gap 1 (grpc-status and grpc-message are trailers)
- Go reference: net/http's TrailerPrefix, Header "Trailer", Request.Trailer, Response.Trailer; httpguts badTrailer; h2 writeChunk (trailers with no body: the response HEADERS, then the trailing HEADERS)
- Test: internal/llvm/testdata/golden/http_trailers (HTTP/1.1 and HTTP/2, native output equal to the Go backend's: want.txt equals want_go.txt)
- Timing: none (no hot path changes; a trailer is a header write after the body)

`BodyWriter.set_trailer`, `Response.with_trailer_names`,
`BodyReader.trailers`, `ClientStream.trailers` and
`Request.with_trailers(names, f)` send and read fields after a body, as
net/http does: names canonical and sorted, Go's forbidden names dropped,
a request's trailers declared in its head (Go's HTTP/2 server keeps only
declared ones), and an HTTP/1.1 streamed response with no body carrying
only the trailers it declared. The Go
backend maps them to net/http. The native wire (Xo Lang 3): the server
starts a chunked response whenever trailers are declared or set (HTTP/2:
a trailing HEADERS frame with END_STREAM), the client keeps a chunked
trailer section and an h2 trailing HEADERS, and a request's trailers go
out as the chunked trailer section or an h2 trailing HEADERS (a streamed
h2 body sends no content-length: 0). A server keeps a request's trailers
only when its Trailer field declared them; the parsed head carries that
field (Req.trailer, H2Req.trailer) since Go removes it from the header.
Names sort with `x = x.sort()` (List.sort returns a new list).

## 2026-10-30: fix: a header field folded over several lines read its first byte wrong on Linux

- Kind: fix
- Commit: this commit
- Found by: internal/llvm TestHTTP1ParseDiff on linux/arm64 (the lead's offload host list, 2026-10-29)
- Go reference: net/http's textproto.Reader.ReadMIMEHeader joins an obsolete line fold with one space
- Test: http1_test.xo "an obsolete line fold joins the lines with one space"; gotchas/raw-address-outlives-bytes.md

The request parser read a folded field's value through a raw address after
the joined text was released, so on Linux the allocator's free-list word
overwrote its first byte ("Accept:a\r\n b" gave "b b"); the text is kept
alive until the value is copied.

## 2026-10-29: perf: a streamed response body is read in large pieces straight into their bytes

- Kind: perf
- Commit: this commit
- Found by: bench/results/httpproxy-2026-10-29.md (the proxy 6x Go's time)
- Go reference: bufio.Reader.Read's fast path (a read of at least the
  buffer's size goes straight into the caller's slice), io.Copy's 32 KiB
  buffer; decision 0404
- Test: http_client_stream, http_reverse_proxy goldens; xo bench --ab in
  bench/results/httpproxy-direct-2026-10-29.md

ClientStream.read reads a piece of 16 KiB or more straight from the
connection into its own bytes when nothing is buffered (one system call, no
copy), and the reverse proxy reads 64 KiB pieces: 1 GiB through the proxy in
187 ms against 581 ms before, and 219 ms for Go's httputil.ReverseProxy.

## 2026-10-29: fix: tls_state no longer reads the TLS connection's state from the handler's task (a data race over HTTP/2)

- Kind: fix
- Commit: this commit
- Found by: internal/llvm TestGoldenTasks/http_tls under ThreadSanitizer, 3 of 3 runs on main after edd6fd640 (Xo Lang 2's bisect): tlssrv load in the handler's task against the connection's write on another thread
- Go reference: crypto/tls's ConnectionState is read under the connection's handshake mutex; Xo records it once instead
- Test: TestGoldenTasks/http_tls and http_tls_state (XO_THREADS 1, 2 and default), TestGoldenRun/http_tls and http_tls_state

`Request.tls_state` (edd6fd640) read the connection's TLS state with
tlssrv.negotiated, which loaded the Conn from the connection's block in
the request's task while an HTTP/2 connection's writer, on another
thread, was moving the same Conn out and back (take_conn, stash).
The handshake now records what it negotiated once, when it completes,
in a third word of the connection's block (an Any, as the server's
SrvTls), before any request's task exists; negotiated reads that word
and never the Conn, and free releases it. The values are the same.

## 2026-10-30: test: the http_tls golden expects url.scheme "https" over TLS (on purpose, unlike Go)

- Kind: test
- Commit: this commit
- Found by: main red on golden http_tls after ea6f13462 (want.txt stale since 53b878858 set the scheme over TLS)
- Go reference: net/http's server leaves URL.Scheme empty on a request it parses (NewServerRequest parses the request target alone); Xo differs on purpose
- Test: golden http_tls (both backends, arm64 and LLVM, Go parity), TestH2Diff

A request served over TLS has `url.scheme` "https" on both backends
(spec std.md, `tls()`: "url.scheme is then https"), whether it came by
HTTP/1.1 (53b878858) or HTTP/2 (ea6f13462); a plain request keeps "".
This differs from Go, whose server leaves URL.Scheme empty, so a handler
that forwards the request or builds a redirect does not have to consult
`tls()` itself. The golden's want.txt was re-recorded; no behavior
changed.

## 2026-10-30: feature: router not_found and method_not_allowed hooks, Allow on a 405, error_html

- Kind: feature
- Commit: this commit
- Found by: the HTTP server gaps survey (part 3): the 404 and 405 were fixed JSON, a 405 carried no Allow
- Go reference: net/http 1.22's ServeMux (Allow on a 405 lists the pattern methods of the path); chi's NotFound and MethodNotAllowed; html/template-free pages as Go's Error gives text
- Test: golden http_router_hooks (both backends)

`r.not_found(h)` and `r.method_not_allowed(h)` answer a miss in place
of the `{"error":"not found"}` and `{"error":"method not allowed"}`
JSON; a hook runs as a handler (inside the middleware, an error a
response, a fault a 500). A 405, default or a hook's without one,
carries `Allow` with the methods of the routes that match the path in
registration order without repeats. A mounted router's hooks are not
copied: its misses are the outer router's. `http.error_html(status,
title, detail)` is an HTML page (text/html, nosniff) with the title in
<title> and <h1> and the detail in <p>, both escaped.
## 2026-10-30: feature: Request.tls_state, what the connection's handshake negotiated

- Kind: feature
- Commit: this commit
- Found by: the HTTP server gaps survey (part 3; Go's r.TLS had only `tls()`)
- Go reference: net/http's Request.TLS (tls.ConnectionState: Version, CipherSuite, ServerName, DidResume, PeerCertificates)
- Test: golden http_tls_state (HTTP/1.1 and HTTP/2 over TLS, plain, a struct literal)

`req.tls_state()` is `Some(TlsState)` over TLS: the version ("TLS 1.2"
or "TLS 1.3"), the suite's name as Go's CipherSuiteName, the server name
the client asked for (SNI; "" for an IP address), whether the handshake
resumed a session, and the client's certificates (std/x509; empty: the
server asks the client for none, as Go's default ClientAuth). None over
plain HTTP and for a request built as a struct literal. Native: the
connection's tlssrv state is read once when the request is built
(tlssrv.negotiated, hidden field tls_neg); Go: from hr.TLS.

## 2026-10-30: fix: an HTTP/2 request's url.scheme is https when the :scheme pseudo header is missing

- Kind: fix
- Commit: this commit
- Found by: TestH2Diff on windows/amd64 (CI run 40926030751: native `scheme=` against Go's `scheme=https`)
- Go reference: net/http's HTTP/2 server fills URL.Scheme from :scheme
- Test: internal/llvm TestH2Diff (the Mac), golden http_server_request

stream_request (std/native/http.xo) takes "https" when h2srv's request
has no scheme; HTTP/2 is reached through with_tls only, so the scheme is
known. The HTTP/1.1 path got the same from the connection's TLS state in
53b878858.
## 2026-10-30: fix: the server's timeouts reach TLS connections (server gaps, part 3 item 1)

- Kind: fix
- Commit: this commit
- Found by: the lead's review of part 2 (a slow client held an HTTPS connection open for ever: the slowloris case)
- Go reference: net/http's ReadTimeout and ReadHeaderTimeout apply to a TLS conn's handshake and reads
- Test: golden http_server_limits (a client that connects to the HTTPS server and sends nothing is closed after the head timeout)

hconn.recv_until gives the TLS connection its deadline (tlssrv.set_deadline)
around each record read, and a timed out wait is the same -2 as a plain
socket's; the handshake, which hserve.hc_h2 runs before the connection's
limits are stored, gets net/http's handshake deadline (the smaller of the
read and head timeouts, http.xo handshake_deadline) and fails past it, so
the connection closes without a reply. Before, a TLS read waited without
a deadline; a first draft that set it only in recv_until left the
handshake unbounded, and the golden's silent client hung the recording
(gotchas/tls-handshake-before-limits.md).

## 2026-10-29: feature: server timeouts and limits (server gaps, part 2)

- Kind: feature
- Commit: this commit
- Found by: std-coverage.md rank 11 (the lead's list)
- Go reference: net/http Server.ReadTimeout, ReadHeaderTimeout, IdleTimeout, MaxHeaderBytes, MaxBytesReader (go1.26.1)
- Test: golden http_server_limits (a 431 over the header cap, a 413 over the body cap and through max_body, a late head closed without a reply), identical on both backends

`with_read_timeout`, `with_read_header_timeout`, `with_idle_timeout`,
`with_max_header_bytes` (431 over it) and `with_max_body` (413 before the
handler, the connection closed) on Server, and the `max_body(n)`
middleware (413 with Go's text). Natively the limits sit on each connection's parse record
(http1.Limits) and the reads take a deadline (hconn.recv_until); the Go
backend sets net/http's fields.

## 2026-10-29: feature: Request host, tls, raw_query, escaped_path; redirect, error_page, not_found (server gaps, part 1)

- Kind: feature
- Commit: this commit
- Found by: std-coverage.md rank 11 (the lead's list)
- Go reference: net/http Request.Host, Request.TLS, URL.RawQuery, URL.EscapedPath, Redirect, Error, NotFound (go1.26.1)
- Test: std/native/http_test.xo (Go's TestRedirect table, Error's text, escaped_path cases), internal/llvm golden http_server_request

`req.host()`, `req.tls()` (and `url.scheme` "https" over TLS, for a proxy's
X-Forwarded-Proto), `req.raw_query()`, `req.escaped_path()`; `http.redirect`
(Go's relative resolution and Location escaping), `http.error_page`,
`http.not_found`. Part 2: the server's timeouts, header and body limits.

## 2026-10-29: feature: WebSocket and other protocol switches through http.reverse_proxy

- Kind: feature
- Commit: this commit
- Found by: decision 0404 stage E (a gateway in front of a WebSocket service)
- Go reference: net/http/httputil reverseproxy.go handleUpgradeResponse,
  upgradeType; decision 0404 stage E
- Test: internal/llvm/testdata/golden/http_reverse_proxy_ws (native and Go)

ReverseProxy.serve keeps a requested `Upgrade` on the outgoing request and, on
the target's 101, hands the client's connection over and copies bytes both ways
until either side ends: a WebSocket echo, a 100 KB binary message, the
subprotocol and a server close pass through. A switch to another protocol than
the one asked for is 502. A client stream on a 101 now reads the connection's
raw bytes (llrt/http1.xo read_head).

## 2026-10-29: feature: a reverse proxy (http.reverse_proxy)

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-29 std survey (std-coverage.md): an API gateway
- Go reference: net/http/httputil reverseproxy.go (ReverseProxy, Rewrite,
  ProxyRequest.SetURL and SetXForwarded, removeHopByHopHeaders,
  joinURLPath/singleJoiningSlash, flushInterval); decision 0404 stage D
- Test: internal/llvm/testdata/golden/http_reverse_proxy (native and Go)

`http.reverse_proxy(client, target)` and `ReverseProxy.serve(req, body)` for a
`handle_stream` route: path joined and query merged as SetURL does, hop-by-hop
fields dropped both ways, X-Forwarded-For/-Host/-Proto set, a `with_rewrite`
hook, both bodies streamed (decision 0404 stages B and C), redirects passed
through, event streams flushed, 502 and 504 on failures. Not yet: Upgrade
tunnels (stage E).

## 2026-10-29: feature: a request body sent as it is read (Request.with_body_reader)

- Kind: feature
- Commit: this commit
- Found by: decision 0404 stage C (a proxy or upload relay had to hold the
  whole request body)
- Go reference: net/http NewRequest with an io.Reader body (ContentLength
  -1, no GetBody: transfer.go chunked writing; client.go redirectBehavior
  returns a 307/308 without GetBody); decision 0404 stage C
- Test: internal/llvm/testdata/golden/http_client_body_reader (native and Go)

`Request.with_body_reader(b)` sends a BodyReader's bytes as they are read,
chunked unless the request gives Content-Length, over HTTP/1.1 and HTTP/2
(DATA frames as the window allows). The body is never sent twice: a 307 or
308 is returned, a 303 is followed as a GET without it.

## 2026-10-29: fix: std builds for separate compilation again after Client.open

- Kind: fix
- Commit: this commit
- Found by: xo-lang-6 (`xo dev stdobj` failed with XO0510 in std/native/http.xo)
- Go reference: none (an Xo checker rule, core 5.3)
- Test: `xo dev stdobj` in the gate; http_client_stream and http_tls goldens

ClientStream.read moved its gzip Decompressor (move only) inside its loop,
which the full check of std rejects; default builds fell back to the
whole-program build. The decoder now moves in a helper outside the loop.

## 2026-10-29: feature: an HTTP/2 response body read as it arrives

- Kind: feature
- Commit: this commit
- Found by: decision 0404 stage B2 (B1 buffered an HTTP/2 body whole)
- Go reference: net/http h2_bundle.go transportResponseBody.Read and
  Close (RST_STREAM CANCEL); decision 0404 stage B2
- Test: internal/llvm/testdata/golden/http_tls (stream big, stream early,
  after early; native and Go)

`Client.open` over https with h2 no longer reads the whole body first:
each `read` returns the next DATA frame's bytes, so memory stays flat as the
body grows. `close` before the end sends RST_STREAM CANCEL and keeps the
connection pooled for the next request.

## 2026-10-29: feature: a response body read as it arrives (Client.open)

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-29 std survey (std-coverage.md): a download, a
  proxy or an event stream had to hold the whole body
- Go reference: net/http Response.Body (transfer.go body, chunked.go
  chunkedReader), Transport's gzip reader; decision 0404 stage B1
- Test: internal/llvm/testdata/golden/http_client_stream (native and Go)

`Client.open(req)` returns a `ClientStream` once the head has arrived:
`head()`, `read(max)`, `read_all(max_size)` and `close()`. A Content-Length,
chunked or until-close body is read in pieces natively (a body cut short is
`Protocol`, `unexpected EOF`), gzip is decoded as it is read, and
`Request.with_idle_timeout(d)` limits each read while the client's timeout
bounds only the head. A body read to its end returns its connection to the
pool; one closed or dropped early closes it. `send` is unchanged. Not yet:
an HTTP/2 response's body is still buffered whole before `open` returns
(stage B2), so memory grows with the body over https with h2.

## 2026-10-29: feature: the client's redirect policy and a request's own time limit

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-29 std survey (std-coverage.md): an API gateway could
  not pass a 3xx through
- Go reference: net/http client.go Client.CheckRedirect (ErrUseLastResponse,
  defaultCheckRedirect), Client.Timeout; decision 0404 stage A
- Test: internal/llvm/testdata/golden/http_client_policy (native and Go)

`Client.with_redirects(max)` follows at most max redirects: 0 returns the 3xx
as it came with its Location untouched (a proxy passes it through), and past
max the request fails with `Protocol` (`stopped after MAX redirects`). The
default stays 10 and now follows 10 (the spec's "up to 10"; Go's
defaultCheckRedirect counts requests and followed 9 here before).
`Request.with_timeout(d)` gives one request its own limit for the whole
exchange in place of its client's. A redirect to another host already
dropped Authorization and cookies; spec 4.5 now says so.

## 2026-10-26: fix: an Expires date outside Time's range has no Expires on the Go backend

- Kind: fix
- Commit: this commit
- Found by: the corpus's range rows on the Go backend (a four digit year
  outside 1678 to 2262)
- Go reference: net/http cookie.go ParseSetCookie, whose time.Time holds any
  year up to 9999; Xo's Time is int64 nanoseconds (decision 0364)
- Test: internal/stdtest/testdata/corpus/http (SET_COOKIE_LINES, go, arm64
  and LLVM)

`xoCookie` (std/rt/cookie.go) converted net/http's Expires with UnixNano,
which wraps a date outside Time's range to a different year, so the Go
backend reported an Expires for "Wed, 16 Dec 9999 09:51:07 GMT" where
native's `cookie_date` has none (time.instant refuses it). A date outside
the range now leaves Expires unset on both. net/http still holds the date,
so the corpus's oracle drops it for Xo. The range's ends are held: "Fri, 01
Jan 1678 00:00:00 GMT" and "Thu, 10 Apr 2262 00:00:00 GMT" both read.

## 2026-10-26: fix: a cookie date's month is read in any case, as Go reads it

- Kind: fix
- Commit: this commit
- Found by: the seeded cookie table (internal/stdtest/testdata/corpus/http) on
  go, arm64 and LLVM
- Go reference: net/http cookie.go readSetCookies through time.Parse's month
  lookup (format.go lookup and match: ASCII case folding, the three letter
  names only); go1.26.1
- Test: internal/stdtest/testdata/corpus/http (COOKIE_LINES, SET_COOKIE_LINES
  and the seeded rows, go, arm64 and LLVM)

`cookie_date` compared the month field with "Jan" to "Dec" exactly, so
"Sun, 29 JUN 2021 22:46:55 GMT" kept no Expires where Go's parser reads June
(Go compares the first three bytes under ASCII case folding). The comparison
now folds case, which matches Go for every spelling: "JUN", "jun" and "juN"
read, and "June" still does not, since Go's lookup matches "Jun" and leaves
the "e" for the layout's next space to refuse. The corpus's cookie tables are
now seeded as well: 2020 Cookie values and 2050 Set-Cookie values built from
every separator, bare token, quoted and escaped value, control byte, and an
Expires in all three of Go's date shapes with every weekday, month and zone
spelling.

## 2026-10-26: fix: a header name with a non-token byte is matched exactly

- Kind: fix
- Commit: this commit
- Found by: the seeded header corpus (internal/stdtest/testdata/corpus/http)
  on go, arm64 and LLVM
- Go reference: net/http Header.Get through textproto.CanonicalMIMEHeaderKey
  (a name with a byte that is not a token is canonicalized to itself, so its
  lookup is exact); go1.26.1
- Test: internal/stdtest/testdata/corpus/http (HEADERS and REPEAT_HEADERS,
  plus "a struct literal's headers read the way the spec says")

`header` (and `header_values`, which reads the same way) canonicalizes the
lookup name and, when that key is absent, still falls back to a
case-insensitive scan of the stored names, which is the documented behavior
(std.md 4.3) and what a struct literal's `headers: {"content-type": ...}`
relies on. The fallback is now limited to lookup names that are RFC tokens:
a name with a byte outside httpguts.ValidHeaderFieldName's set matches the
canonical key only, as Go's Header.Get reads it, since Go's canonical name
for such a name is the name itself. So with the field `"\tType"` stored,
`header("\tTYPE")` is None as in Go, while `header("CONTENT-TYPE")` still
reads a field stored as `content-type`. The HTTP/1.1 and HTTP/2 parsers
canonicalize a received name already, so they read the same.

## 2026-10-26: fix: a cookie date's weekday and zone are checked as Go checks them

- Kind: fix
- Commit: this commit
- Found by: the cookie corpus's follow-up rows, xo-lang-4's review of the
  Expires fix below
- Go reference: net/http cookie.go readSetCookies with the cookie TimeFormat,
  through time.Parse's weekday and zone parsing; cookie_zone_len is a port of
  parseTimeZone, parseGMT and parseSignedOffset (go1.26.1
  src/time/format.go, over bytes as Go does)
- Test: internal/stdtest/testdata/corpus/http (SET_COOKIE_LINES, go, arm64 and
  LLVM)

`cookie_date` read the weekday without looking at it and the zone without
looking at it either, so "Xyz, 09 Jun 2021 10:18:14 GMT" and
"Wed, 09 Jun 2021 10:18:14 gmt" both became expiry dates. Go leaves Expires
unset for both, and so does Xo now. The weekday is one of the seven three
letter names in any case ("wed" and "WED" read). The zone is what Go's parser
reads in full, ported from the standard library: three to five capitals where
a fourth or fifth must end in T ("UTC", "CEST", "AKDST"), a fourth may also be
"WITA", "ChST" and "MeST" are the only zones with a lower case letter, "GMT"
takes a sign and an hour of 23 or less ("GMT+8", "GMT-23"), and a bare sign is
an hour of 23 or less with nothing after it ("+05", "+0023"). A lowercase
"gmt", a numeric "123", a bare sign in front of four digits ("+0100", 100
hours), "GMT+24", "UTC+8", "ABCD", and a non ASCII zone all leave Expires
unset, exactly as in Go. The time itself is still read as UTC.

## 2026-10-26: fix: an Expires date in the standard cookie format is no longer dropped

- Kind: fix
- Commit: this commit
- Found by: the new cookie corpus (internal/stdtest/testdata/corpus/http) on
  arm64 and LLVM; the Go backend never had the bug
- Go reference: net/http cookie.go readSetCookies, which parses Expires with
  the cookie TimeFormat (go1.26.1)
- Test: internal/stdtest/testdata/corpus/http (SET_COOKIE_LINES, go, arm64 and
  LLVM)

`parse_set_cookie` read an Expires value through cookie_date, which split the
date on spaces and accepted four fields after the weekday or three. Go's cookie
TimeFormat leaves five ("02 Jan 2006 15:04:05 GMT"), so every standard
Set-Cookie date returned None and the cookie lost its expiry: a value handed to
a browser again kept a session cookie that should have expired. The hyphenated
form with a four digit year ("02-Jan-2006 15:04:05 GMT") still reads, as it does
in Go, whose parser is lenient about the separator; a two digit year (RFC 850), a
date with no zone, and the ANSIC form leave Expires unset, which is also what Go
does.

## 2026-10-23: fix: http.new_request's path on the Go backend

- Kind: fix
- Commit: this commit
- Found by: TestStdCorpora on the Go backend (added the same day; before it a -run with /go/ matched nothing)
- Go reference: net/url URL.Path and URL.EscapedPath
- Test: internal/stdtest/testdata/corpus/httpurl on the Go backend

`http.new_request` set `req.path` to the loose parse's path on the Go
backend: "/" for a URL with no path (`http://www.google.com`), the escaped
form for a path with escapes (`/a%20b`). The native backends, and Go, give
"" for no path (and for an opaque URL), the decoded path otherwise, and
`%XX` only for bytes that are not UTF-8 (`/é/%FF`). The Go backend now
does the same. `req.url.path` was already right.

## 2026-10-23: behavior: HttpErr, ClientErr, UrlErr, CookieErr and TlsErr print plain messages (decision 0358)

- Kind: behavior
- Commit: this commit
- Found by: the Intel field report on 0.26.0 (error texts nested Debug output)
- Go reference: net/http, net/smtp and text/template error texts
- Test: golden error_text_http (arm64, LLVM, Go parity)

`Display` printed the Debug form (`Connect(msg: "...")`, `UrlErr(msg: ...)`).
It prints plain text now: `HttpErr` the message of an `Io` and `canceled`;
`ClientErr` `invalid URL: MSG`, MSG for a `Connect`, `request timed out`,
`protocol error: MSG`, `canceled`; `UrlErr`, `CookieErr` and `TlsErr` their
message. `Debug` and value comparison are unchanged; a program that
matches the old text must change.

## 2026-10-23: test: parse_url and new_request against Go's net/url

- Kind: test
- Commit: this commit
- Found by: Go's url_test.go inputs and 4,000 seeded random URLs and 1,200 queries run through url.Parse and ParseQuery
- Go reference: net/url url_test.go (Copyright 2009 The Go Authors, BSD-style license), url.go (go1.26.1)
- Test: internal/stdtest/testdata/corpus/httpurl (TestStdCorpora, arm64 and LLVM)

The corpus compares parse_url (scheme, host, port, path, first value of each
query key, and its three error texts) and new_request (path, url path, query)
with Go. It found the postgres multi-host gap fixed in std/url.

## 2026-10-23: perf: query_parts reads a plain query in place

- Kind: perf
- Commit: this commit
- Found by: XO_PROF=cpu of http.new_request (query_parts was 18% of it)
- Go reference: net/url url.go parseQuery (go1.26.1)
- Test: std/native/http_test.xo "a plain query is read in place", internal/stdtest/testdata/corpus/url (TestStdCorpora), goldens http_form and http_router_paths

`http1.query_parts` counted the `&` bytes one at a time, searched each pair
with a string find, and ran every key and value through `neturl.unescape`
(a byte loop and a UTF-8 check each). It now finds `&` with
`scan.find_byte`, checks for a `;` once, and when the whole query has no
`%` or `+` and is valid UTF-8 (three word-at-a-time scans) every key and
value is a view of the query. `xo bench --ab`, 300k `new_request` calls on
four URLs with plain queries: 0.756 instructions and 0.821 wall against the
old code (156.6 to 128.7 ms), and against Go's `http.NewRequest` with
`URL.Query()` 0.827 instructions, wall within noise, peak memory 0.17;
the mixed row of 400k calls (two queries, one with `%20` and one with
`+`): 0.937 instructions, 0.98 wall, Go 0.722 instructions, 0.704 wall.
bench/results/query-parts-2026-10-23.md.

## 2026-10-23: perf: new_request and parse_url parse the URL once, and url_parse scans a word at a time

- Kind: perf
- Commit: this commit
- Found by: XO_PROF=cpu of http.new_request (url_parts was 47% of it, called twice)
- Go reference: net/url url.go Parse (go1.26.1); http.NewRequest
- Test: internal/stdtest/testdata/corpus/url (TestStdCorpora), golden http_router

`new_request` and `parse_url` called `http1.url_parts` twice for one URL;
they now share one result. `neturl.url_parse` finds the control-byte check
with `scan.ctl_at`, the query with `scan.find_byte`, and the path's escape
with `scan.find_byte` instead of per-byte loops, and the raw query is a
view. `xo bench --ab` of 400k `new_request` calls on four URLs: 2.94 G to
2.06 G instructions (0.699) and 164 ms to 114 ms for the parse-once change,
then 2.06 G to 1.83 G (0.890, wall 0.882) for the scans. Against Go's
`http.NewRequest` the same row is 0.779 of its instructions and 0.838 of
its wall time (bench/results/url-parse-2026-10-23.md).

## 2026-10-23: perf: a router's path parameters skip the range clamp

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 3's router dispatch row (three routes, 200,000 dispatches) against Go's ServeMux
- Go reference: net/http ServeMux.PathValue (a substring of the path)
- Test: none (same output; the dispatch row prints the same checksum before and after)

`bind_in` binds a parameter with `raw.str_cut` (the bounds are the segment
bounds already checked) instead of `raw.str_slice`, which clamped both ends
again. Dispatch of a prepared request: 905 M to 864 M instructions,
`xo bench --ab` instructions 0.954 (interval 0.954 to 0.955), wall within
noise (0.968, n=10).

## 2026-10-22: perf: the HTTP date layout is parsed once

- Kind: perf
- Commit: 8ddf1bbe0
- Found by: bench/results/time-format-cache-variants-2026-10-22.md (the layout parse was 62% of a text-layout format)
- Go reference: none (Go's time.Format has no parsed layout type; this is the lazy const of decision 0332)
- Test: golden http_cookies (the Expires text), http_serve_content (Last-Modified)

`Cookie.display`'s Expires and the Last-Modified header format their date with
a lazy `const HTTP_DATE_LAYOUT: time.Layout` instead of parsing the layout
text on every call. Set-Cookie with an Expires over 300,000 cookies: 3,079 M
to 1,320 M instructions, `xo bench --ab` wall 0.427x (interval 0.403 to 0.465),
the same output.

## 2026-10-22: perf: a server's connection fibers no longer hold 2 MiB each of resident memory

- Kind: perf
- Commit: d08537802
- Found by: bench/perf/netgo comparison (CRUD server peak RSS at 1024 connections: Xo 2.1 to 2.3 GB, Go 66 to 78 MB)
- Go reference: none (goroutine stacks start small)
- Test: none (a memory figure of the Linux runtime: /proc/PID/smaps, gotchas/fiber-stacks-huge-pages.md)

Each fiber has a 64 MiB reserved stack. On a Linux with transparent huge
pages set to `always` (Docker Desktop's VM and the default of several
distributions) the first touch of a stack's top faulted in a whole 2 MiB
page, so every live connection cost 2 MiB of resident memory. The runtime
now marks fiber stacks MADV_NOHUGEPAGE: the CRUD server at 1024
connections holds 47 to 57 MB (64 connections: 21 MB against 158 MB), and
its throughput did not change within the run to run noise.

## 2026-10-21: perf: a large streamed write goes out as one chunk

- Kind: perf
- Commit: f0aae00e5
- Found by: bench/perf/tlsrate (TLS bulk over HTTP/1.1: Xo 220 MB/s against Go's 2,000 MB/s on the same TLS code; HTTP/2 downloads were at parity)
- Go reference: net/http response.Write through bufio.Writer.Write (a write larger than the buffer's room goes around an empty buffer)
- Test: internal/llvm/testdata/golden/http_stream (a 3000 byte write: one chunk bb8, not 800 and 3b8)

BodyWriter.write was bufio's WriteString: it filled a 2 KiB buffer and
sent a chunk (head, data, CRLF: three TLS records over TLS) per 2 KiB, so
a body written in 64 KiB pieces made 32 chunks each. It now follows
Write: a write larger than the room left goes out whole when the buffer
is empty (HTTP/1.1 one chunk, HTTP/2 DATA frames of the same data).

## 2026-10-21: fix: a streamed body's read error sticks

- Kind: fix
- Commit: b5d9c4c66
- Found by: review after the websocket close fix (the same Mutex.with pattern)
- Go reference: net/http body (a read error is returned again)
- Test: internal/llvm/testdata/golden/http_stream_body (bad chunk), http_multipart_stream

On the native backends, `BodyReader.read` after a failed read, and
`PartReader.next` or `read` after a failed one, read the connection
again instead of returning the same error.

## 2026-10-21: feature: BodyReader.multipart (multipart parts from the stream)

- Kind: feature
- Commit: 942699fbf
- Found by: decision 0310 phase 3
- Go reference: net/http Request.MultipartReader; mime/multipart Reader.NextPart and Part.Read; mime/quotedprintable (go1.26.1)
- Test: internal/llvm/testdata/golden/http_multipart_stream (native identical to the Go backend)

A `handle_stream` route can read a `multipart/form-data` body part by
part as it arrives: `b.multipart(req)` returns a `PartReader` whose
`next()` gives each part's name, file name, and headers
(`header_values` for repeated ones) and whose `read(max)` gives the
part's data, so an upload is never held in memory whole.

## 2026-10-21: feature: streamed request bodies over HTTP/2

- Kind: feature
- Commit: 8ac517e64
- Found by: decision 0310 phase 2
- Go reference: golang.org/x/net/http2 server (requestBody, the stream's inflow returned as the handler reads, RST_STREAM NO_ERROR for an unread body; go1.26.1's bundled h2_bundle.go)
- Test: internal/llvm/testdata/golden/http_stream_body_h2 (300,000 byte bodies over a 64 KiB stream window, small reads, a body read halfway and one not read; native identical to the Go backend)

A `handle_stream` route on an HTTPS server now reads an HTTP/2 request
body as its DATA frames arrive: the native server hands the request
over at its HEADERS and gives the stream's window back as the handler
reads, as net/http's.

## 2026-10-21: feature: Router.handle_stream (streamed request bodies)

- Kind: feature
- Commit: f2358842
- Found by: owner's request (2026-10-21), decision 0310
- Go reference: net/http's Request.Body (body, chunkedReader, expectContinueReader, chunkWriter.writeHeader's discard; go1.26.1)
- Test: internal/llvm/testdata/golden/http_stream_body (17 exchanges: Content-Length and chunked bodies read 3 bytes at a time, in parts as they arrive, read_all, unread bodies small and large, 100-continue read and unread, malformed chunks; native identical to the Go backend)

`r.handle_stream(pattern, fn(req, body) { ... })` calls the handler
before the body is read; `body.read(max)` and `body.read_all()` read it
as it arrives, as Go's `r.Body`. The native HTTP/1.1 server reads the
connection's input on demand, sends 100 Continue on the first read, and
after the handler discards an unread rest under 256 KiB or closes.

## 2026-10-21: feature: http.gzip_responses

- Kind: feature
- Commit: 4cddcc7b
- Found by: decision 0305 (approved by the owner, 2026-10-21)
- Go reference: none in Go's standard library; the rules of klauspost/compress gzhttp
- Test: internal/llvm/testdata/golden/http_gzip_responses

`r.wrap(http.gzip_responses())` compresses responses with gzip when the
request accepts it. It skips small bodies, compressed content types,
206s, and responses that already have a Content-Encoding. It adds
`Vary: Accept-Encoding`, weakens ETags, and compresses streamed bodies
through a Compressor that each flush reaches.
## 2026-10-21: feature: http.serve_content

- Kind: feature
- Commit: 5ff295e6
- Found by: owner's request (2026-10-21)
- Go reference: net/http ServeContent, serveContent, checkPreconditions, scanETag, parseRange (go1.26.1)
- Test: internal/llvm/testdata/golden/http_serve_content (35 requests: ranges, conditional requests, HEAD, content types; the native backends match the Go backend's net/http byte for byte, the multipart boundary aside)

`http.serve_content(req, name, modtime, content, headers)` returns the
response Go's ServeContent writes: Range requests (one range as 206
with Content-Range, several as multipart/byteranges), If-Match,
If-Unmodified-Since, If-None-Match, If-Modified-Since and If-Range
against modtime and the ETag in `headers`, 416 for a range outside the
content, and the Content-Type from `headers`, the name's extension, or
the content (decision 0306).

## 2026-10-21: feature: Request.client_ip behind trusted proxies

- Kind: feature
- Commit: b37efc91
- Found by: survey (client addresses behind load balancers; with Xo Lang 3)
- Go reference: net/netip (Prefix.Contains, Addr.Unmap); RFC 7239; the walk of chi's and gorilla's proxy headers middleware
- Test: internal/llvm/testdata/golden/http_client_ip (every backend)

`client_ip(trusted, header)`: the peer, or the rightmost address of the
named proxy header outside the trusted prefixes, with only that header
read and the first untrusted hop ending the walk (None when it is hidden,
unknown, or does not parse; reviewed with Xo Lang 3).

## 2026-10-21: feature: the client keeps HTTP/1.1 connections alive

- Kind: feature
- Commit: 04ea3e47
- Found by: per-line profile of bench/perf/httpclient (Xo Lang 3)
- Go reference: net/http Transport (getConn, tryPutIdleConn, persistConn.readLoop, shouldClose, isReplayable, shouldRetryRequest; go1.26.1)
- Test: internal/llvm/testdata/golden/http_client_keepalive (connections a raw server accepted, request by request, as the Go backend's); TestHTTP1ClientDiff (457 responses, llvm, go, arm64)

Natively, an HTTP/1.1 connection (plain or TLS) goes back to a pool
after a response read in full that leaves it open, and the next request
to the same scheme, address and server name takes it: at most 2 idle
per host, 100 in all, 90 s idle, as Transport's defaults. A response
with Connection: close, an HTTP/1.0 one without keep-alive, a body that
ends at EOF, or a 101 closes it; a connection the server closed while
idle is dropped when taken, and a replayable request that met a dead
pooled connection is sent once more on a new one. The client bench's
HTTP/1.1 workloads against a keep-alive server: 1,119M to 520M
instructions for 3000 requests (decision 0303).

## 2026-10-21: fix: the native client reads responses as net/http does

- Kind: fix
- Commit: c9dad995
- Found by: differential test against Go
- Go reference: net/http ReadResponse, readTransfer (parseTransferEncoding, fixLength, fixTrailer, shouldClose), internal/chunked.go; net/textproto readMIMEHeader (go1.26.1)
- Test: internal/llvm/testdata/golden/http_client_responses (349 raw responses: net/http's parsing cases and 300 random header blocks; arm64, LLVM, and the Go backend agree); internal/llvm TestHTTP1ClientDiff's 457 random responses now agree with the Go backend on every one

The native HTTP/1.1 client was lenient where Go's is strict, and
reported other texts. It took a bad or negative Content-Length, two
different ones, or one like `2, 2`; a Transfer-Encoding other than
chunked; a field line without a colon, with an empty or invalid name,
or with a control byte; and kept only the first line of a folded field.
Each is now Go's error (`net/http: HTTP/1.x transport connection
broken: bad Content-Length "x"`, `unsupported transfer encoding:
"gzip"`, `malformed MIME header line: ...`), a folded field is joined
with a space, equal Content-Length fields read as one, a closing
HTTP/1.1 response has no `Connection` field (as net/http removes it),
and the body's errors (`unexpected EOF`, `invalid byte in chunk length`)
come without the request, as Go's body reads give them. An error text
keeps a zero byte of the line it quotes (it was cut there), a status
line's bytes are quoted as Go's %q (`"\xff"`), and a Location that
url.Parse rejects is Client.Do's `Get "URL": failed to parse Location
header ...`. On the Go backend, an error text quoting a field line with
bytes that are not UTF-8 now has U+FFFD for them (decision 0259), as
natively. TestHTTP1ClientDiff now holds every backend to the Go
backend's output on all 457 of its responses (testdata/http1_client.txt
is the Go backend's).

## 2026-10-21: speedup: Router matching without a string per path segment

- Kind: speedup
- Commit: 73612e39
- Found by: per-line callgrind of bench/perf/http (Xo Lang 3)
- Go reference: none
- Test: internal/llvm/testdata/golden/http_router_paths (124 method and path cases, the same answers as before the change)

Natively, a request path without escapes is matched against the routes
where it is: its segments' offsets are found once and each route's
literals compared in place, so no segment strings or unescaped copies
are made (a path with escapes is matched as before). The plain HTTP
server's router code took 177 fewer instructions a request (callgrind,
arm64).

## 2026-10-21: fix: a request's repeated Set-Cookie fields

- Kind: fix
- Commit: a6a1e92a
- Found by: differential test against Go
- Go reference: net/http request parsing (readRequest, textproto ReadMIMEHeader) as the Go backend's fieldView shows it (go1.26.1)
- Test: internal/llvm/testdata/golden/http_header_fields (a request with two Set-Cookie fields); 467 raw requests (net/textproto and net/http parsing cases and 400 random header blocks) gave the same status, header Map, and body natively as on the Go backend

The native servers (HTTP/1.1 and HTTP/2) joined a request's repeated
Set-Cookie fields with ", " in `headers`; decision 0288 keeps the first,
as the Go backend and the client do (`header_values` gives each).

## 2026-10-21: fix: parse_url's host and fragment checks and error texts

- Kind: fix
- Commit: b2ef88de
- Found by: differential test against Go
- Go reference: net/url Parse, parseHost, unescape (encodeHost, encodeZone), setFragment (go1.26.1)
- Test: std/native/http_test.xo (1,586 URLs: url_test.go's inputs and 1,500 seeded random ones, with the Go backend's results)

Natively, `parse_url` accepted 11 URLs Go rejects (a bad escape in the
fragment, which Go's setFragment checks) and rejected an IPv6 literal
with an escaped zone Go accepts (`[fe80::1%25%65%6e%30]`, RFC 6874), and
every bad host was `invalid host "..."` instead of Go's reason (`invalid
port ":0x" after host`, `missing ']' in host`, `invalid character " "
in host name`, netip's text for a bad IPv6 literal). The native host
parser is now a port of Go's parseHost; the same texts reach an x509
certificate's URI name errors.

## 2026-10-20: feature: header fields with repeated names; fix: Set-Cookie

- Kind: feature
- Commit: 33fec016
- Found by: survey
- Go reference: net/http Header.Add, Header.Values, SetCookie, Response.Cookies (go1.26.1)
- Test: internal/llvm/testdata/golden/http_header_fields (HTTP/1.1 and HTTP/2; arm64, LLVM, and the Go backend agree)

`header_values` and `add_header` on Request and Response, and
`Response.with_cookie` and `cookies` (decision 0288, part 2): several
fields of one name are sent and received each. A fix with it: a client
response with several `Set-Cookie` fields was one value joined with ", "
(which `Expires` dates also contain); `headers` now shows the first, and
`header_values` or `cookies()` give all. The HTTP/2 client also kept the
last of a repeated field in the Map instead of joining them.

## 2026-10-20: feature: cookies

- Kind: feature
- Commit: 54da082a
- Found by: survey
- Go reference: net/http/cookie.go Cookie.String, ParseCookie, ParseSetCookie, readCookies, Request.Cookie, Cookies, AddCookie (go1.26.1)
- Test: internal/llvm/testdata/golden/http_cookies (the inputs of Go's cookie_test.go tables; arm64, LLVM, and the Go backend's net/http agree); std/native/http_test.xo (Go's TestCookieSanitizeValue, TestCookieSanitizePath, writeSetCookiesTests' domains and names)

`http.Cookie` with `display()` (the Set-Cookie value), `parse_cookie`,
`parse_set_cookie`, and `Request.cookie`, `cookies`, and `with_cookie`
(decision 0288, part 1). Bytes and domains Go drops with a log line are
dropped silently.
## 2026-10-20: change: form() parses the Content-Type as Go does

- Kind: change
- Commit: 11afca47
- Found by: review (Xo Lang 3)
- Go reference: net/http/request.go parsePostForm, TestParseFormUnknownContentType (go1.26.1)
- Test: std/native/http_test.xo (go TestParseFormUnknownContentType); internal/llvm/testdata/golden/http_form

`req.form()` reads the media type with `mime.parse_media_type`, so a
malformed Content-Type (`text/plain; boundary=`) is now an error with
mime's text, where it was an empty form; a well formed one behaves as
before.

## 2026-10-20: port: Request.multipart_form

- Kind: port
- Commit: 4de48c2e
- Found by: survey
- Go reference: net/http/request.go ParseMultipartForm, multipartReader (go1.26.1)
- Test: internal/llvm/testdata/golden/multipart_basic

`req.multipart_form()` reads a `multipart/form-data` body as a
`multipart.Form` with Go's Content-Type checks and error texts (decision
0282).

## 2026-10-20: fix: the client sends a URL's query as given

- Kind: fix
- Commit: 361a427c
- Found by: survey
- Go reference: net/http/client.go, net/url URL.RawQuery (go1.26.1)
- Test: internal/llvm/testdata/golden/http_form (repeated keys and their order); internal/llvm/testdata/golden/http_tls (HTTP/2)

The client rebuilt a request's query from `url.query`, a map holding one
value per key, so `client.get("http://h/s?q=a&q=b")` sent only `q=a`. It
now sends the query as given unless `url.query` was changed.

## 2026-10-20: feature: Request.form and Request.query_all

- Kind: feature
- Commit: e2be349f
- Found by: survey
- Go reference: net/http/request.go ParseForm, PostForm; net/url URL.Query (go1.26.1)
- Test: std/native/http_test.xo (Go's TestParseFormQuery, TestParseFormQueryMethods, TestParseFormUnknownContentType, TestParseFormSemicolonSeparator); internal/llvm/testdata/golden/http_form

`req.form()` parses an `application/x-www-form-urlencoded` body into
`url.Values`, and `req.query_all(name)` returns every value of a repeated
query key; the server keeps each request's raw query for it, over HTTP/1.1
and HTTP/2.

## 2026-11-06: feature: Client.with_server_name, with_verify_conn, tls_options and Request.with_host

- Kind: feature
- Commit: this commit
- Found by: Xo Azure 1 (azidentity's Service Fabric thumbprint pin and the workload identity token proxy), decision 0541
- Go reference: tls.Config.ServerName and VerifyConnection, http.Request.Host
- Test: golden tls_verify_conn (go, arm64, llvm)
- Shipped in: not yet released

A client can set the TLS server name (SNI and the verified name), check the server's certificates after the handshake and before
any request byte, and send a Host that differs from the URL's. A client with a server name or hook has its own pooled connections.

## 2026-11-07: feature: Client.with_verify, and with_roots keeps the server name and hook

- Kind: feature
- Commit: this commit
- Found by: Xo Azure 1 (azidentity's Service Fabric endpoint: self signed, known only by its SHA-1 thumbprint), decision 0541
- Go reference: tls.Config.InsecureSkipVerify with VerifyConnection
- Test: golden tls_verify_conn (go, arm64, llvm)
- Shipped in: not yet released

`client.with_verify(tls.Verify.None)` skips the chain and name checks, and `ChainOnly` skips the name; a `None` client without a
`with_verify_conn` pin refuses every connection. `tls_options()` reports the mode. `with_roots` called after `with_server_name`,
`with_verify_conn` or `with_verify` no longer drops them.

## 2026-11-07: fix: an HTTP/2 request's host() is its :authority

- Kind: fix
- Commit: this commit
- Found by: golden tls_verify_conn (decision 0541): the native HTTP/2 server answered "" for Request.host()
- Go reference: net/http's Request.Host is the :authority of an HTTP/2 request
- Test: golden tls_verify_conn
- Shipped in: not yet released

The native server took host() from the target's URL host, empty for an ordinary request, instead of the :authority (or Host) it had parsed.

## 2026-11-07: security: with_verify, with_verify_conn, with_server_name and with_cert did nothing on wasm for a default client

- Kind: security
- Commit: this commit
- Found by: lead tier 57, TestWasmCommandGoldens tls_verify_conn (decision 0541): on wasm the Verify.None and pin cases answered "certificate refused"
- Go reference: net/http Transport with a tls.Config (InsecureSkipVerify, VerifyConnection, ServerName)
- Test: golden tls_verify_conn on wasm ("none wrong pin" fails with a pin mismatch and delivers nothing, as on native)
- Shipped in: not yet released (main only, from 4f1c83115 to this commit; no release holds it)

A client with the default roots has no roots record on wasm, and the four Client methods returned it unchanged. A `with_verify_conn` pin or hook set on such a client was therefore never enforced, which fails open (the connection kept the default full verification, so it was not weaker than the default, but the extra check the caller asked for did not run), and a server name or Verify.None was ignored. They now start from the system's roots with no certificates, which is what a native default client reports.
