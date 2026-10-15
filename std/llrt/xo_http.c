// xo_http.c: the C glue of the native std/http (decision 0074), included
// by xo_std.c.
//
// std/http is Xo since the zero-C audit's B28 part 3 (2026-10-14): the
// HTTP/1.1 connection (std/native/llrt/http1.xo, http1w.xo), the HTTP/2
// server (h2srv.xo), the server's registration, connections, sinks and
// streamed responses (hconn.xo, hserve.xo), and the client (hclient.xo).
// TLS is Xo too (decision 0202; Mbed TLS was removed on 2026-10-15).
// What stays C: the weak stand-ins of cabi_http.xo's exports, which the
// services record names, and the forwarders of rt.Any (anyval.xo) and of
// a std function's fault, which generated code calls by these names.

// cabi_http.xo's exports (std/http's programs carry them; the weak
// definitions keep other programs linking, which never make a server, a
// connection, a sink, or a client response, and have no streamed
// responses or HTTP/2 connections to free at exit)
__attribute__((weak)) void xo_x_srv_free(int64_t p) { (void)p; }
__attribute__((weak)) void xo_x_hc_free(int64_t p) { (void)p; }
__attribute__((weak)) void xo_x_sink_free(int64_t p) { (void)p; }
__attribute__((weak)) void xo_x_cresp_free(int64_t p) { (void)p; }
__attribute__((weak)) void xo_x_streams_free(void) {}

// Values of any type (request values, std.md 4.3; anyval.xo) and a std
// function's fault at its caller: generated code calls these names.
int64_t xo_x_any_new(int64_t l, int64_t drop, int64_t tag, int64_t tn);
int64_t xo_x_any_get(int64_t a, int64_t tag, int64_t tn);
void xo_x_fault_at(int64_t p, int64_t n, int64_t lp, int64_t ln);

void *xo_any_new(void *l, int64_t drop, const char *tag, int64_t tn) { return (void *)(intptr_t)xo_x_any_new(XO_ADDR(l), drop, XO_ADDR(tag), tn); }

xo_rlist *xo_any_get(void *a, const char *tag, int64_t tn) { return (xo_rlist *)(intptr_t)xo_x_any_get(XO_ADDR(a), XO_ADDR(tag), tn); }

XO_NORETURN void xo_fault_at(const char *msg, int64_t n, const char *loc, int64_t ln) {
	xo_x_fault_at(XO_ADDR(msg), n, XO_ADDR(loc), ln);
	abort();
}
