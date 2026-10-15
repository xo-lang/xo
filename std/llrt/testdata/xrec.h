// xrec.h: the task runtime's records as the C tests see them. The
// records are Xo's (std/native/llrt pool.xo, ctx.xo, tasks.xo, chan.xo,
// selects.xo, mutex.xo, budget.xo), and xo_rt.c keeps no copy of their
// layouts (the zero-C inventory's rank 13, 2026-10-15): here they are
// opaque, with the few words the tests read at the Xo modules' offsets.
// Include after ../xo_rt.c (xcalls.h includes it).
#ifndef XO_XREC_H
#define XO_XREC_H

typedef struct xo_task xo_task;
typedef struct xo_scope xo_scope;
typedef struct xo_chan xo_chan;
typedef struct xo_select xo_select;
typedef struct xo_mutex xo_mutex;
typedef struct xo_atomic xo_atomic;
typedef struct xo_ctx xo_ctx;
typedef struct xo_park xo_park;
typedef struct xo_node xo_node;

// a lock word (pool.xo: 0 free, 1 held, 2 held and a thread may sleep),
// taken and released as generated code and the Xo code do
typedef struct {
	int64_t v;
} xo_lock;
void xo_x_futex_wake(int64_t a);
void xo_x_lock_slow(int64_t l);
static inline void lk(xo_lock *l) {
	int64_t z = 0;
	if (!__atomic_compare_exchange_n(&l->v, &z, 1, 0, __ATOMIC_ACQUIRE, __ATOMIC_RELAXED)) {
		xo_x_lock_slow(XO_ADDR(l));
		(void)__atomic_load_n(&l->v, __ATOMIC_ACQUIRE);
	}
}
static inline void unlk(xo_lock *l) {
	if (__atomic_exchange_n(&l->v, 0, __ATOMIC_RELEASE) == 2) {
		xo_x_futex_wake(XO_ADDR(&l->v));
	}
}

// a task's state (tasks.xo T_STATE, sched.xo's states) and result
// (T_RESULT: a shared list of one when it ran to the end)
enum { T_QUEUED, T_RUNNING, T_DONE };
#define T_STATE_OFF 80
#define T_RESULT_OFF 112
static inline int64_t t_task_state(xo_task *t) { return __atomic_load_n((int64_t *)((char *)t + T_STATE_OFF), __ATOMIC_ACQUIRE); }
static inline xo_rlist *t_task_result_of(xo_task *t) { return *(xo_rlist **)((char *)t + T_RESULT_OFF); }

// the current context (ctx.xo cur) and its boundary word (C_BOUNDARY: the
// frame address a fault's unwind never passes)
int64_t xo_x_ctx_cur(void);
#define C_BOUNDARY_OFF 112
static inline void t_ctx_boundary(uintptr_t frame) { *(uintptr_t *)((char *)(intptr_t)xo_x_ctx_cur() + C_BOUNDARY_OFF) = frame; }

#endif
