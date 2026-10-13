// pthread.h for Windows builds of the TLS library (decision 0197): the
// subset Mbed TLS's MBEDTLS_THREADING_PTHREAD and xo/*.c use, on SRW
// locks, condition variables, and one-time initialization. Only Windows
// compiles put xo/win on the include path (internal/build tls.go).
#ifndef XO_WIN_PTHREAD_H
#define XO_WIN_PTHREAD_H

#ifndef WIN32_LEAN_AND_MEAN
#define WIN32_LEAN_AND_MEAN
#endif
#include <windows.h>

typedef SRWLOCK pthread_mutex_t;
typedef CONDITION_VARIABLE pthread_cond_t;
typedef INIT_ONCE pthread_once_t;
#define PTHREAD_MUTEX_INITIALIZER SRWLOCK_INIT
#define PTHREAD_ONCE_INIT INIT_ONCE_STATIC_INIT

static __inline int pthread_mutex_init(pthread_mutex_t *m, const void *a)
{
    (void) a;
    InitializeSRWLock(m);
    return 0;
}
static __inline int pthread_mutex_destroy(pthread_mutex_t *m)
{
    (void) m;
    return 0;
}
static __inline int pthread_mutex_lock(pthread_mutex_t *m)
{
    AcquireSRWLockExclusive(m);
    return 0;
}
static __inline int pthread_mutex_trylock(pthread_mutex_t *m)
{
    return TryAcquireSRWLockExclusive(m) ? 0 : 16; // EBUSY
}
static __inline int pthread_mutex_unlock(pthread_mutex_t *m)
{
    ReleaseSRWLockExclusive(m);
    return 0;
}

static __inline int pthread_cond_init(pthread_cond_t *c, const void *a)
{
    (void) a;
    InitializeConditionVariable(c);
    return 0;
}
static __inline int pthread_cond_destroy(pthread_cond_t *c)
{
    (void) c;
    return 0;
}
static __inline int pthread_cond_signal(pthread_cond_t *c)
{
    WakeConditionVariable(c);
    return 0;
}
static __inline int pthread_cond_broadcast(pthread_cond_t *c)
{
    WakeAllConditionVariable(c);
    return 0;
}
static __inline int pthread_cond_wait(pthread_cond_t *c, pthread_mutex_t *m)
{
    return SleepConditionVariableSRW(c, m, INFINITE, 0) ? 0 : 22; // EINVAL
}

static BOOL CALLBACK xo_pthread_once_tramp(PINIT_ONCE o, PVOID fn, PVOID *ctx)
{
    (void) o;
    (void) ctx;
    ((void (*)(void)) fn)();
    return TRUE;
}
static __inline int pthread_once(pthread_once_t *o, void (*fn)(void))
{
    InitOnceExecuteOnce(o, xo_pthread_once_tramp, (PVOID) fn, NULL);
    return 0;
}

#endif
