#!/bin/bash
# gen.sh SRC_TXZ RELEASE: writes internal/repotest/testdata/freebsd-abi.txt,
# the FreeBSD ABI facts std/native/llrt/freebsd.xo uses (decision 0573):
# system call numbers from sys/sys/syscall.h, and constants and struct
# offsets computed by clang for x86_64 and aarch64 from the release's own
# headers. SRC_TXZ is the release's src.txz, checked against its MANIFEST
# from download.freebsd.org (fetch both from
# https://download.freebsd.org/releases/amd64/RELEASE/). Run from the
# repository root; needs curl, tar and clang (any clang with the X86 and
# AArch64 targets; nothing is linked). TestFreeBSDABI compares the file
# with freebsd.xo.
set -euo pipefail
src=${1:?src.txz}
rel=${2:?release, e.g. 14.5-RELEASE}
out=internal/repotest/testdata/freebsd-abi.txt
work=$(mktemp -d)
trap 'rm -rf "${work:?}"' EXIT
want=$(curl -sSfL "https://download.freebsd.org/releases/amd64/$rel/MANIFEST" | awk '$1 == "src.txz" { print $2 }')
got=$(shasum -a 256 "$src" | awk '{ print $1 }')
if [ -z "$want" ] || [ "$want" != "$got" ]; then
  echo "gen.sh: $src has sha256 $got; the $rel MANIFEST says '$want'" >&2
  exit 1
fi
tar -xf "$src" -C "$work" usr/src/sys/sys usr/src/sys/amd64/include usr/src/sys/arm64/include usr/src/sys/x86/include usr/src/include
t=$work/usr/src/sys
for a in amd64 arm64; do
  mkdir -p "$work/inc-$a"
  ln -s "$t/sys" "$work/inc-$a/sys"
  ln -s "$t/$a/include" "$work/inc-$a/machine"
  ln -s "$t/x86/include" "$work/inc-$a/x86"
done
# the facts, as NAME EXPR; freebsd.xo's constant of NAME must equal EXPR
facts='ST_MODE offsetof(struct stat, st_mode)
ST_MTIM offsetof(struct stat, st_mtim)
ST_SIZE offsetof(struct stat, st_size)
ST_SIZEOF sizeof(struct stat)
TP_START_FUNC offsetof(struct thr_param, start_func)
TP_ARG offsetof(struct thr_param, arg)
TP_STACK_BASE offsetof(struct thr_param, stack_base)
TP_STACK_SIZE offsetof(struct thr_param, stack_size)
TP_TLS_BASE offsetof(struct thr_param, tls_base)
TP_TLS_SIZE offsetof(struct thr_param, tls_size)
TP_CHILD_TID offsetof(struct thr_param, child_tid)
TP_PARENT_TID offsetof(struct thr_param, parent_tid)
TP_FLAGS offsetof(struct thr_param, flags)
TP_RTP offsetof(struct thr_param, rtp)
TP_SIZE sizeof(struct thr_param)
MAP_PRIVATE MAP_PRIVATE
MAP_ANON MAP_ANON
O_RDONLY O_RDONLY
O_WRONLY O_WRONLY
O_RDWR O_RDWR
O_APPEND O_APPEND
O_CREAT O_CREAT
O_TRUNC O_TRUNC
O_EXCL O_EXCL
O_CLOEXEC O_CLOEXEC
AT_FDCWD AT_FDCWD
AT_SYMLINK_NOFOLLOW AT_SYMLINK_NOFOLLOW
CLOCK_REALTIME CLOCK_REALTIME
CLOCK_MONOTONIC CLOCK_MONOTONIC
UMTX_OP_WAIT_UINT_PRIVATE UMTX_OP_WAIT_UINT_PRIVATE
UMTX_OP_WAKE_PRIVATE UMTX_OP_WAKE_PRIVATE
EINTR EINTR
EAGAIN EAGAIN
KEV_IDENT offsetof(struct kevent, ident)
KEV_FILTER offsetof(struct kevent, filter)
KEV_FLAGS offsetof(struct kevent, flags)
KEV_FFLAGS offsetof(struct kevent, fflags)
KEV_DATA offsetof(struct kevent, data)
KEV_UDATA offsetof(struct kevent, udata)
KEV_SIZE sizeof(struct kevent)
EVFILT_READ EVFILT_READ
EVFILT_WRITE EVFILT_WRITE
EVFILT_USER EVFILT_USER
EV_ADD EV_ADD
EV_DELETE EV_DELETE
EV_ENABLE EV_ENABLE
EV_DISABLE EV_DISABLE
EV_ONESHOT EV_ONESHOT
EV_CLEAR EV_CLEAR
EV_EOF EV_EOF
EV_ERROR EV_ERROR
NOTE_TRIGGER NOTE_TRIGGER
SA_HANDLER offsetof(struct sigaction, sa_handler)
SA_FLAGS_AT offsetof(struct sigaction, sa_flags)
SA_MASK offsetof(struct sigaction, sa_mask)
SA_SIZE sizeof(struct sigaction)
SA_ONSTACK SA_ONSTACK
SA_RESTART SA_RESTART
SA_SIGINFO SA_SIGINFO
SIGBUS SIGBUS
SIGSEGV SIGSEGV
SIGPIPE SIGPIPE
SIGURG SIGURG
SIGCHLD SIGCHLD
SIGPROF SIGPROF
SIG_BLOCK SIG_BLOCK
SIG_SETMASK SIG_SETMASK
SS_SIZE_AT offsetof(stack_t, ss_size)
SS_FLAGS_AT offsetof(stack_t, ss_flags)
SS_SIZEOF sizeof(stack_t)
UC_MCONTEXT offsetof(ucontext_t, uc_mcontext)
MC_SIZEOF sizeof(mcontext_t)'
{
  echo '#include <sys/types.h>'
  for h in sys/stat.h sys/thr.h sys/umtx.h sys/mman.h sys/fcntl.h sys/errno.h sys/time.h sys/event.h sys/signal.h sys/ucontext.h stddef.h; do echo "#include <$h>"; done
  echo 'long abi[] = {'
  echo "$facts" | while read -r _ e; do echo "  $e,"; done
  echo '};'
} > "$work/abi.c"
vals() {
  clang -target "$1-unknown-freebsd${rel%%-*}" -nostdinc -isystem "$work/inc-$2" -isystem "$work/usr/src/include" \
    -isystem "$(clang -print-resource-dir)/include" -S -emit-llvm -o - "$work/abi.c" |
    sed -n 's/^@abi = .*\[\(i64 .*\)\], align.*/\1/p' | tr ',' '\n' | sed 's/ *i64 //'
}
vals x86_64 amd64 > "$work/amd64"
vals aarch64 arm64 > "$work/arm64"
{
  echo "# FreeBSD $rel ABI facts for std/native/llrt/freebsd.xo (decision 0573),"
  echo "# from src.txz sha256 $got (its MANIFEST's), by scripts/freebsdabi/gen.sh."
  echo "# NAME AMD64 ARM64; SYS_name lines are sys/sys/syscall.h's numbers."
  echo "$facts" | awk '{ print $1 }' | paste -d' ' - "$work/amd64" "$work/arm64"
  awk '$1 == "#define" && $2 ~ /^SYS_/ && $3 ~ /^[0-9]+$/ { print $2, $3, $3 }' "$t/sys/syscall.h"
} > "$out"
echo "gen.sh: wrote $out ($(wc -l < "$out") lines)"
