# The arm64 MIR retargeted to x86: where arm64's semantics leak

Status: Observed (2026-10-17, decision 0254 stage 2)

The amd64 backend translates the arm64 front half's templates
(selfhost/arm64 x86.xo). Each of these cost a debugging round; each
looked like a correct translation instruction by instruction.

## arm64's sdiv does not trap; x86's idiv does

Symptom: const_div died with SIGFPE before printing a line (stdout is
buffered). Cause: arm64's sdiv gives 0 for a division by zero and the
minimum for minimum / -1, so the front half may speculate a division (an
if conversion runs both arms); x86's idiv raises #DE on both. Fix: the
x86 sdiv branches around idiv for -1 (a neg) and 0 (zero).

## One scratch register for two jobs in parallel moves

Symptom: a struct swap (`let t = a; a = b; b = t`) of spilled values
corrupted the heap's free list (a later halloc faulted). Cause: arm64
breaks a move cycle in x16 and moves memory to memory through x17; the
x86 emitter had only r11 for both, so a memory move inside a cycle
overwrote the cycle's saved value. Fix: memory to memory is `pushq src;
popq dst` (a pop to an rsp relative address computes it after the
increment, so both offsets are rsp's before the push).

## The calling convention's arguments and results differ on x86

Symptom: maps' update counts were 0. Cause: shrink.xo tail_call checked
the arguments with abi_locs, which on arm64 is the arguments' convention
too (x0 to x7 both ways) but on x86 is the results' (eight registers,
the arguments have six); a call with two stack arguments became a tail
call whose arguments died with the frame. Fix: call_locs for the
arguments.

## Flags between instructions

x86 writes flags in instructions arm64 does not (sub, and, shifts, xor
for zero). A carry chain cut into one instruction per link (x86_split,
for x86's 13 registers) keeps its carry only because nothing but moves
comes between the links: the emitter zeroes with `movl $0`, never xor.

## Debugging linux/amd64 binaries from this Mac

gdb and ptrace fail under Docker's amd64 emulation ("Couldn't get
registers"). What works: an arm64 container (scripts/docker-linux.sh
arm64, XO_DOCKER_ROOT=1) with qemu-user and gdb-multiarch, the static
binary under `qemu-x86_64 -g PORT` and gdb attached with `set
architecture i386:x86-64`; software watchpoints only step the selected
thread, and the mapping's addresses change from run to run, so pin a
watchpoint with `eval "watch *(long*)0x%lx", expr`. XO_LEAKCHECK=1 on
both backends for the same program (linux/arm64 cross built and run in
Docker) narrowed a reference count bug to one function in minutes.
