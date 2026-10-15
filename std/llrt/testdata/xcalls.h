// xcalls.h: the calls generated code makes into the task runtime's Xo
// exports (std/native/llrt tasks.xo, chan.xo, selects.xo, mutex.xo),
// typed for the C tests that play generated code.
// Include after ../xo_rt.c.

int64_t xo_x_task_spawn(int64_t s, int64_t fl, int64_t run, int64_t drop, int64_t fdrop, int64_t flags, int64_t site, int64_t lp, int64_t ln);
int64_t xo_x_scope_open(void);
int64_t xo_x_scope_close(int64_t s, int64_t mode);
void xo_x_scope_cancel(int64_t s);
int64_t xo_x_scope_fail_task(int64_t s);
void xo_x_task_done(int64_t t, int64_t rl, int64_t failed);
void xo_x_task_unwound(int64_t t);
int64_t xo_x_task_await(int64_t t, int64_t lp, int64_t ln);
int64_t xo_x_task_result(int64_t t);
void xo_x_chan_send(int64_t c, int64_t l, int64_t lp, int64_t ln);
int64_t xo_x_chan_recv(int64_t c);
int64_t xo_x_chan_try_send(int64_t c, int64_t l);
int64_t xo_x_chan_try_recv(int64_t c);
int64_t xo_x_chan_len(int64_t c);
int64_t xo_x_select_new(void);
int64_t xo_x_select_wait(int64_t s, int64_t lp, int64_t ln);
int64_t xo_x_mutex_new(int64_t l, int64_t drop);
int64_t xo_x_mutex_lock(int64_t m, int64_t lp, int64_t ln);
void xo_x_mutex_unlock(int64_t m, int64_t l);
int64_t xo_x_mutex_rlock(int64_t m, int64_t lp, int64_t ln);
void xo_x_mutex_runlock(int64_t m, int64_t l);

static xo_task *t_task_spawn(xo_scope *s, xo_rlist *fl, int64_t run, int64_t drop, int64_t fdrop, int64_t flags, int64_t site, const char *loc, int64_t n) {
	return (xo_task *)(intptr_t)xo_x_task_spawn(XO_ADDR(s), XO_ADDR(fl), run, drop, fdrop, flags, site, XO_ADDR(loc), n);
}
static xo_scope *t_scope_open(void) { return (xo_scope *)(intptr_t)xo_x_scope_open(); }
static int64_t t_scope_close(xo_scope *s, int64_t mode) { return xo_x_scope_close(XO_ADDR(s), mode); }
static void t_scope_cancel(xo_scope *s) { xo_x_scope_cancel(XO_ADDR(s)); }
static xo_task *t_scope_fail_task(xo_scope *s) { return (xo_task *)(intptr_t)xo_x_scope_fail_task(XO_ADDR(s)); }
static void t_task_done(xo_task *t, xo_rlist *rl, int64_t failed) { xo_x_task_done(XO_ADDR(t), XO_ADDR(rl), failed); }
static void t_task_unwound(xo_task *t) { xo_x_task_unwound(XO_ADDR(t)); }
static xo_rlist *t_task_await(xo_task *t, const char *loc, int64_t n) { return (xo_rlist *)(intptr_t)xo_x_task_await(XO_ADDR(t), XO_ADDR(loc), n); }
static xo_rlist *t_task_result(xo_task *t) { return (xo_rlist *)(intptr_t)xo_x_task_result(XO_ADDR(t)); }
static xo_chan *t_chan_new(int64_t cap, int64_t drop, const char *loc, int64_t n) { return (xo_chan *)(intptr_t)xo_x_chan_new(cap, drop, XO_ADDR(loc), n); }
static void t_chan_send(xo_chan *c, xo_rlist *l, const char *loc, int64_t n) { xo_x_chan_send(XO_ADDR(c), XO_ADDR(l), XO_ADDR(loc), n); }
static xo_rlist *t_chan_recv(xo_chan *c) { return (xo_rlist *)(intptr_t)xo_x_chan_recv(XO_ADDR(c)); }
static int64_t t_chan_try_send(xo_chan *c, xo_rlist *l) { return xo_x_chan_try_send(XO_ADDR(c), XO_ADDR(l)); }
static xo_rlist *t_chan_try_recv(xo_chan *c) { return (xo_rlist *)(intptr_t)xo_x_chan_try_recv(XO_ADDR(c)); }
static void t_chan_close(xo_chan *c) { xo_x_chan_close(XO_ADDR(c)); }
static int64_t t_chan_len(xo_chan *c) { return xo_x_chan_len(XO_ADDR(c)); }
static xo_select *t_select_new(void) { return (xo_select *)(intptr_t)xo_x_select_new(); }
static int64_t t_select_wait(xo_select *s, const char *loc, int64_t n) { return xo_x_select_wait(XO_ADDR(s), XO_ADDR(loc), n); }
static xo_mutex *t_mutex_new(xo_rlist *l, int64_t drop) { return (xo_mutex *)(intptr_t)xo_x_mutex_new(XO_ADDR(l), drop); }
static xo_rlist *t_mutex_lock(xo_mutex *m, const char *loc, int64_t n) { return (xo_rlist *)(intptr_t)xo_x_mutex_lock(XO_ADDR(m), XO_ADDR(loc), n); }
static void t_mutex_unlock(xo_mutex *m, xo_rlist *l) { xo_x_mutex_unlock(XO_ADDR(m), XO_ADDR(l)); }
static xo_rlist *t_mutex_rlock(xo_mutex *m, const char *loc, int64_t n) { return (xo_rlist *)(intptr_t)xo_x_mutex_rlock(XO_ADDR(m), XO_ADDR(loc), n); }
static void t_mutex_runlock(xo_mutex *m, xo_rlist *l) { xo_x_mutex_runlock(XO_ADDR(m), XO_ADDR(l)); }

// The counted objects' entries generated code reached through C before
// the 0.12.0 seed: obj.xo's and rc.xo's exports, and the leak check's
// sums (heapos.xo).
void xo_x_obj_rc_defer_n(int64_t obj, int64_t drop);
int64_t xo_x_obj_rs_concat(int64_t a, int64_t an, int64_t b, int64_t bn);
void xo_x_obj_rl_free_n(int64_t l, int64_t esize, int64_t drop);
void xo_x_obj_rl_share(int64_t l, int64_t esize, int64_t share);
void xo_x_heap_live(int64_t live, int64_t total);
int64_t xo_x_atomic_new(int64_t v);
int64_t xo_x_ctx_unwinding(void);
int64_t xo_x_rc_claim(int64_t p);
int64_t xo_x_rc_mark(int64_t p);
void xo_x_obj_rc_free(int64_t p);
void xo_x_obj_h_free(int64_t p);

static int rc_claim(int64_t *p) { return (int)xo_x_rc_claim(XO_ADDR(p)); }
static int rc_mark(int64_t *p) { return (int)xo_x_rc_mark(XO_ADDR(p)); }

static void *t_rc_alloc(int64_t n) { return xo_alloc(n); }
static void t_rc_free(void *p) { xo_x_obj_rc_free(XO_ADDR(p)); }
static void t_rc_defer(void *obj, xo_drop_fn drop) { xo_x_obj_rc_defer_n(XO_ADDR(obj), XO_ADDR(drop)); }
static xo_rstr *t_rs_concat(xo_rstr *a, int64_t an, xo_rstr *b, int64_t bn) { return (xo_rstr *)(intptr_t)xo_x_obj_rs_concat(XO_ADDR(a), an, XO_ADDR(b), bn); }
static void t_rl_free(xo_rlist *l, int64_t esize, xo_elem_fn drop) { xo_x_obj_rl_free_n(XO_ADDR(l), esize, XO_ADDR(drop)); }
static void t_rl_share(xo_rlist *l, int64_t esize, xo_elem_fn share) { xo_x_obj_rl_share(XO_ADDR(l), esize, XO_ADDR(share)); }
static void t_rt_live(int64_t *live, int64_t *total) { xo_x_heap_live(XO_ADDR(live), XO_ADDR(total)); }
static xo_atomic *t_atomic_new(int64_t v) { return (xo_atomic *)(intptr_t)xo_x_atomic_new(v); }
static int64_t t_unwinding(void) { return xo_x_ctx_unwinding(); }
