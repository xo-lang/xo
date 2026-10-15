// xo_http.c: the C glue of the native std/http (decision 0074), included
// by xo_std.c.
//
// std/http is Xo since the zero-C audit's B28 part 3 (2026-10-14): the
// HTTP/1.1 connection (std/native/llrt/http1.xo, http1w.xo), the HTTP/2
// server (h2srv.xo), the server's registration, connections, sinks and
// streamed responses (hconn.xo, hserve.xo), and the client (hclient.xo).
// What stays C is TLS (xo_tls.c on Mbed TLS, decision 0105, included at
// the end; the zero-C audit's B31): the calls Xo makes to it
// (rthttp.tls_c_* and cl_tls_*, below) and the start of the server
// record that xo_srv_tls sets; the weak stand-ins of cabi_http.xo's
// exports, which the services record names; and the forwarders of rt.Any
// (anyval.xo) and of a std function's fault, which generated code calls
// by these names.

// The results of socket operations of the client (and of TLS ones;
// hclient.xo CC_*)
enum { CC_OK, CC_EOF, CC_TIMEOUT, CC_CANCELED, CC_ERR };

// TLS (xo_tls.c, included at the end of this file; decision 0105)
typedef struct tls_conn tls_conn;
typedef struct tls_srv tls_srv;
typedef struct xo_troots xo_troots;
static int64_t tls_read(tls_conn *t, char *p, int64_t n);
static int tls_write(tls_conn *t, const char *p, int64_t n);
static void tls_conn_free(tls_conn *t);
static void tls_close_notify(tls_conn *t);
static void tls_srv_free(tls_srv *s);
static tls_conn *tls_server_conn(tls_srv *s, xo_pd *pd);
static int64_t tls_hc_read(tls_conn *t, char *p, int64_t n);
static int tls_client_start(xo_pd *pd, const char *host, xo_troots *roots, int64_t when, tls_conn **out, char *msg, size_t mn, int *kind);
static const char *tls_alert(tls_conn *t);
static int tls_server_handshake(tls_conn *t);
static int64_t tls_nb_read(tls_conn *t, char *p, int64_t n);
static int tls_alpn_h2(tls_conn *t);
static int tls_h2_bad_cipher(tls_conn *t, char *why, size_t wn);
static void tls_set_when(tls_conn *t, int64_t when);

// The start of a server's record (hconn.xo SR_*): its TLS configuration,
// which xo_srv_tls sets.
typedef struct xo_srv {
	xo_hdr h;
	tls_srv *tls;
} xo_srv;
_Static_assert(offsetof(xo_srv, tls) == 16, "hconn.xo SR_TLS");

#define TC(t) ((tls_conn *)(intptr_t)(t))

// The TLS calls of hconn.xo, hserve.xo and hclient.xo (rthttp.xo).
int64_t xo_tls_c_server(int64_t s, int64_t pd) { return XO_ADDR(tls_server_conn((tls_srv *)(intptr_t)s, (xo_pd *)(intptr_t)pd)); }

int64_t xo_tls_c_hc_read(int64_t t, int64_t p, int64_t n) { return tls_hc_read(TC(t), (char *)(intptr_t)p, n); }

int64_t xo_tls_c_handshake(int64_t t) { return tls_server_handshake(TC(t)); }

int64_t xo_tls_c_alpn_h2(int64_t t) { return tls_alpn_h2(TC(t)); }

int64_t xo_tls_c_bad_cipher(int64_t t, int64_t p, int64_t n) { return tls_h2_bad_cipher(TC(t), (char *)(intptr_t)p, (size_t)n); }

int64_t xo_tls_c_nb_read(int64_t t, int64_t p, int64_t n) { return tls_nb_read(TC(t), (char *)(intptr_t)p, n); }

void xo_tls_c_close_notify(int64_t t) { tls_close_notify(TC(t)); }

void xo_tls_c_srv_free(int64_t s) { tls_srv_free((tls_srv *)(intptr_t)s); }

int64_t xo_tls_c_client(int64_t pd, int64_t host, int64_t roots, int64_t when, int64_t out, int64_t msg, int64_t mn, int64_t kind) {
	tls_conn *t = NULL;
	int k = *(int *)(intptr_t)kind;
	int r = tls_client_start((xo_pd *)(intptr_t)pd, (const char *)(intptr_t)host, (xo_troots *)(intptr_t)roots, when, &t, (char *)(intptr_t)msg,
	                         (size_t)mn, &k);
	*(int64_t *)(intptr_t)out = XO_ADDR(t);
	*(int *)(intptr_t)kind = k;
	return r;
}

int64_t xo_cl_tls_read(int64_t t, int64_t p, int64_t n) { return tls_read(TC(t), (char *)(intptr_t)p, n); }

int64_t xo_cl_tls_write(int64_t t, int64_t p, int64_t n) { return tls_write(TC(t), (const char *)(intptr_t)p, n); }

void xo_cl_tls_free(int64_t t) { tls_conn_free(TC(t)); }

void xo_cl_tls_set_when(int64_t t, int64_t when) { tls_set_when(TC(t), when); }

xo_rstr *xo_cl_tls_alert(int64_t t, int64_t *len) {
	const char *a = tls_alert(TC(t));
	return nstr(a ? a : "", a ? (int64_t)strlen(a) : 0, len);
}

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

// TLS (decision 0105)
#include "xo_tls.c"
