// xo_http.c: the C glue of the native std/http (decision 0074), included
// by xo_std.c.
//
// std/http is Xo since the zero-C audit's B28 part 3 (2026-10-14): the
// HTTP/1.1 connection (std/native/llrt/http1.xo, http1w.xo), the HTTP/2
// server (h2srv.xo), the server's registration, connections, sinks and
// streamed responses (hconn.xo, hserve.xo), and the client (hclient.xo).
// TLS is Xo too (decision 0202; Mbed TLS was removed on 2026-10-15).
// What stays C: the weak stand-ins of cabi_http.xo's exports, which the
// services record names, and the forwarder of a std function's fault,
// which generated code calls by its name (rt.Any is anyval.xo's exports
// directly).

// cabi_http.xo's exports (std/http's programs carry them; the weak
// definitions keep other programs linking, which never make a server, a
// connection, a sink, or a client response, and have no streamed
// responses or HTTP/2 connections to free at exit)
__attribute__((weak)) void xo_x_srv_free(int64_t p) { (void)p; }
__attribute__((weak)) void xo_x_hc_free(int64_t p) { (void)p; }
__attribute__((weak)) void xo_x_sink_free(int64_t p) { (void)p; }
__attribute__((weak)) void xo_x_cresp_free(int64_t p) { (void)p; }
__attribute__((weak)) void xo_x_streams_free(void) {}

// A std function's fault at its caller: generated code calls this name.
void xo_x_fault_at(int64_t p, int64_t n, int64_t lp, int64_t ln);

XO_NORETURN void xo_fault_at(const char *msg, int64_t n, const char *loc, int64_t ln) {
	xo_x_fault_at(XO_ADDR(msg), n, XO_ADDR(loc), ln);
	abort();
}
