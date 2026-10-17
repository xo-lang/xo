# 0253: the architecture of `xo debug`

Status: Proposed (for the owner's review before it is built; Xo lang 3,
2026-10-17, written around the design the owner chose through the lead)

## Context

The owner wants Xo to have its own debugger, written in Xo, working on
every OS Xo targets, so an agent or a person can stop a program, step it,
and read its values and its tasks, with nothing to install. Decision 0252
gives native builds DWARF types, variables, and locations (line tables
since 0239); lldb and gdb read them, but neither knows Xo's tasks,
scopes, or values (a `List[Int]` is a pointer to a header to them), and
lldb is not on every machine (Windows, minimal Linux images).

What Xo already has to build on:

- std/native/llrt/dwline.xo reads line tables and the inlined frames of
  `.debug_info` (DWARF 4 and 5), from the running image or a dSYM, for
  the profiler and tracebacks.
- unwind.xo walks the unwind tables (the profiler's and the traceback's
  walk, fiber_walk for a parked fiber from its saved stack pointer).
- The runtime's records describe every task (decision 0251): the root
  contexts' registry (ctx.xo add_root, through the service record of
  static word SVC), each context's park reason (C_WHY, decision 0240)
  and park frame pointer (C_PARK_FP), each task record's context, state,
  and spawn location, a parked fiber's saved stack pointer. `xo dump`
  walks them from inside the program; a debugger can walk the same
  memory from outside.

## Decision

`xo debug` is an out of process debugger written in Xo. The design
below is the owner's (2026-10-17); the alternatives are recorded as
rejected.

1. **Process control: a thin per OS layer** (selfhost/debug/proc_*.xo),
   with one interface: start a program stopped, attach to a pid, read and
   write its memory, read and write a thread's registers, list its
   threads, set and clear a hardware or software breakpoint, single step
   a thread, resume, and wait for the next stop (breakpoint, step,
   signal, exit). Breakpoints and stepping go through the kernel: no code
   patching from inside the program, nothing in the program's runtime.
   - macOS arm64 first: Mach task and exception ports (task_for_pid,
     mach_vm_read_overwrite and mach_vm_write, thread_get_state and
     thread_set_state with ARM_THREAD_STATE64, task_threads, an exception
     port for EXC_BREAKPOINT and EXC_SOFTWARE, ptrace(PT_ATTACHEXC) to
     take the process's exceptions, posix_spawn with
     POSIX_SPAWN_START_SUSPENDED to start one). Software breakpoints are
     `brk #0` words written through mach_vm_protect and mach_vm_write
     into the stopped process's text (the kernel's copy on write, as
     lldb does; the program never writes its own code, so its W^X rules
     are untouched); single step is the debug state's MDSCR_EL1.SS bit
     (ARM_DEBUG_STATE64).
   - Linux next: ptrace (PTRACE_TRACEME through a fork, PTRACE_ATTACH or
     PTRACE_SEIZE, PTRACE_PEEKDATA/POKEDATA or process_vm_readv,
     PTRACE_GETREGSET/SETREGSET with NT_PRSTATUS, PTRACE_CONT,
     PTRACE_SINGLESTEP, waitpid).
   - Windows later: the debug API (CreateProcess with
     DEBUG_ONLY_THIS_PROCESS, DebugActiveProcess, WaitForDebugEvent and
     ContinueDebugEvent, ReadProcessMemory and WriteProcessMemory,
     GetThreadContext and SetThreadContext with the trap flag for single
     step), from PE/COFF and its DWARF (the LLVM backend writes DWARF on
     Windows too) or later a PDB reader.

2. **Shared parts, written once in Xo** (selfhost/debug/):
   - A DWARF reader: dwline.xo grows to types, variables, location lists
     and expressions (decision 0252's subset: base types, structures,
     DW_OP_reg/regx/breg/piece/fbreg, DW_LLE_start_end/offset_pair/
     base_addressx with addr_base, the forms both backends and dsymutil
     write), from the executable (Linux, Windows) or its dSYM (macOS),
     matched by build id or LC_UUID.
   - Breakpoints at FILE:LINE (every address of the line from the line
     table, inlined copies included) and at a function name; step (into
     calls, by line), next (over calls: a breakpoint at the return
     address and the frame's stack pointer to tell recursion apart),
     finish (the caller's return address), continue.
   - print: a variable or a field path (`print p.a.x`), with values read
     through the location lists, and Xo's own layouts shown as Xo shows
     them: Str as text (its object and length), List as its elements,
     Map and Set as their entries, enums by their variant (the tag word
     and the payload), Box by its value. The layouts are the ones
     decision 0252 documents; a layout change updates the debugger and
     its test in the same commit.
   - backtrace: the unwind tables (unwind.xo's walk, from the stopped
     thread's registers), with inlined frames from the DWARF and each
     frame's variables on request.

3. **Task awareness from the runtime's own records, read from the
   target's memory.** The debugger finds the static words through a
   DWARF global variable that both backends emit for them (`xo_static`,
   added with this decision), then the service record (static word
   SVC), the root contexts' registry (decision 0251), and from each root
   down the cancellation nodes, as dump.xo does. `tasks` lists every task
   with its state and why it is blocked (C_WHY's reason: receive, send,
   select, Mutex.with, await, sleep, network I/O) and where; `task N bt`
   backtraces a parked task from its fiber's saved stack pointer
   (fiber_walk's frame layout) or a waiting root from C_PARK_FP. No agent
   thread, no socket, no code in the program: a hung or crashed process
   is debugged the same way, and a release build is debuggable when it
   keeps its debug information (`--release --debug-info`). dump.xo and
   the debugger share the record layouts through one Xo module of
   constants (ctx.xo's and pool.xo's offsets), so a change of a record
   is one change.

4. **Commands** (`xo debug PROG [ARGS]`, `xo debug -p PID`, building PROG
   with debug information first when it is a package): `break`
   (`b main.xo:12`, `b sum`), `delete`, `run`, `continue` (`c`), `step`
   (`s`), `next` (`n`), `finish`, `print` (`p`), `locals`, `backtrace`
   (`bt`), `frame N`, `tasks`, `task N` (and `task N bt`), `threads`,
   `quit`; a `--batch` mode that runs commands from arguments or a file
   and prints, for agents and tests (`xo debug --batch -c 'b main.xo:12'
   -c run -c locals PROG`), and `--json` output of stops, values, and
   tasks. Help, its golden, the spec's command list, and toolchain.md
   come with the command.

5. **macOS signing.** Debugging another process needs the
   `com.apple.security.cs.debugger` entitlement on `xo debug` (that is,
   on the xo binary): signed with the owner's Developer ID for releases,
   ad hoc for development (the owner runs `DevToolsSecurity -enable`
   once, as for lldb). The program being debugged needs
   `get-task-allow`: selfhost/macho's ad hoc signer adds it to builds
   with debug information that are not release builds (development
   builds), never to release builds unless asked.

6. **Later: a remote mode.** `xo debug --remote HOST:PORT` speaks to an
   `xo debug --serve` running next to the program (in a container, on a
   device), which does the process control; the DWARF and the views stay
   on the client. Not in the first stages.

7. **What other debuggers still get.** The DWARF of decisions 0239 and
   0252 is standard: lldb, gdb, Visual Studio's and WinDbg's DWARF
   support, Instruments, and perf keep reading lines, frames, and
   locals (as structures of words for Xo's collections). Nothing here
   takes that away; their tests stay as cross checks.

8. **Writing variables (`set`) comes after the read only stages.** The
   arm64 emitter keeps a spilled value in x16 or x17 across instructions
   of a block and into a block that follows its single predecessor
   (emit.xo sc_slot, Xo lang 4, 2026-10-17; gotcha
   arm64-scratch-reload-cache), so a write to a spilled variable's stack
   slot can go unseen until the next call or join while the code uses the
   register copy. Reading is unaffected: the slot and the register agree.
   When `set` is built it has the emitter describe the cache: a location
   list entry for each range where x16 or x17 holds the variable's slot
   (DWARF 5 allows overlapping entries: both places are valid there),
   and the write updates every place the word lives at the stop's pc. A
   build without those entries gets no write to a spilled word, with a
   message naming the reason. Values in registers and the LLVM backend's
   locations (LLVM describes its own copies) are writable from the start
   of `set`.

## New OS capabilities (for the owner's approval, rule 7b)

These are calls the debugger's process control layer makes (std/native
syscall and libSystem bindings, reached only by `xo debug`, not by
programs):

- macOS: task_for_pid, task_threads, thread_get_state and
  thread_set_state (ARM_THREAD_STATE64, ARM_DEBUG_STATE64),
  mach_vm_read_overwrite, mach_vm_write, mach_vm_protect,
  mach_port_allocate and mach_port_insert_right, task_set_exception_ports,
  mach_msg (receiving exception messages and replying), ptrace
  (PT_ATTACHEXC, PT_DETACH, PT_KILL), posix_spawn with
  POSIX_SPAWN_START_SUSPENDED, task_resume and thread_resume, kill
  (SIGSTOP, SIGKILL), waitpid; and the entitlement of point 5.
- Linux: ptrace (TRACEME, ATTACH, SEIZE, INTERRUPT, PEEKDATA, POKEDATA,
  GETREGSET, SETREGSET, CONT, SINGLESTEP, DETACH), process_vm_readv,
  fork and execve (or posix_spawn with a traced child), waitpid,
  tgkill; reading /proc/PID/maps and /proc/PID/exe.
- Windows: CreateProcessW (DEBUG_ONLY_THIS_PROCESS),
  DebugActiveProcess and DebugActiveProcessStop, WaitForDebugEventEx,
  ContinueDebugEvent, ReadProcessMemory, WriteProcessMemory,
  FlushInstructionCache, GetThreadContext, SetThreadContext,
  SuspendThread, ResumeThread, OpenProcess, OpenThread.

## Stages

1. selfhost/debug: the DWARF reader (types, variables, locations) over
   dwline.xo, tested against decision 0252's goldens (the values lldb
   and gdb print, read without them).
2. macOS arm64 process control; `xo debug --batch` with break, run,
   continue, locals, print, backtrace on a stopped program; get-task-allow
   in development builds; the debugger entitlement ad hoc.
3. step, next, finish; inlined frames; Str, List, Map, Set, enums.
4. tasks: the static words' DWARF variable, the record walk, `tasks`,
   `task N bt`, attach to a hung process (`-p`); shared constants with
   dump.xo.
5. The interactive prompt and `--json`.
6. Linux ptrace (arm64, then x86_64); then Windows; then the remote mode.

## Consequences

- One debugger for every OS, in Xo, with the toolchain: no lldb or gdb
  to install, and views (tasks, Xo values) no general debugger gives.
- The xo binary on macOS carries an entitlement and needs signing for
  releases (the owner's Developer ID); development builds of programs
  carry get-task-allow, release builds do not.
- The runtime's record layouts and Xo's value layouts become an interface
  of the debugger: each change updates it and its tests.
- No cost in programs: nothing runs in the debugged process.

## Alternatives (rejected)

- An in-runtime debug agent in debug builds, driven over a socket, stopping
  tasks through the scheduler, with breakpoints by compiler-inserted
  checks or code patching: costs every debug build a thread and checks,
  differs from release code (against 0252's same-code gate), cannot stop a
  hung or crashed runtime, and code patching meets macOS arm64's W^X rules
  inside the process.
- A hybrid (an agent for task views, the OS for breakpoints): two
  mechanisms to keep in step; reading the records from outside gives the
  task views without an agent.
- Shipping an lldb Python formatter and an lldb-launching `xo debug`
  (0252's first draft): depends on lldb being installed and does not run
  on Windows or minimal Linux images; superseded by this decision before
  it was built.
- Writing the debugger in Go or C: against the self-hosted toolchain
  (decision 0215).

## Implementation

- Stage 1 (2026-10-17): selfhost/debug dwarf.xo reads the debug sections
  of an ELF file or of a Mach-O file (a dSYM's DWARF file) from its
  bytes, then every DWARF 4 and 5 compile unit: functions with their
  address ranges, their variables and parameters (lexical blocks
  included), the calls inlined into them (ranges from DW_AT_ranges and
  .debug_rnglists, depth, call line) with their variables (names, types,
  and lines from the abstract origin), global variables, and types (base,
  structure with members, pointer, typedef). A location is an exprloc or
  a .debug_loclists list (loclistx through DW_AT_loclists_base, or an
  offset; every DW_LLE kind, addrx through DW_AT_addr_base), decoded into
  pieces of registers, register plus offset, frame base plus offset,
  addresses, constants (DW_OP_lit, constu, consts with stack_value), or
  optimized out. `xo dev dwarf FILE` prints what it reads
  (toolchain.md). Tests: selfhost/debug dwarf_test.xo (expressions, a
  hand made unit with a location list; internal/selfhost
  TestSelfhostDebugXoTests) and internal/arm64 TestDwarfReader (the arm64
  backend's dSYM through selfhost/macho and dsymutil, every variable's
  ranges equal to dwarfdump's; the LLVM backend's, sum inlined with k the
  constant 10). Not yet: the xo_static global (it comes with stage 4,
  where the task views read it), compressed ELF sections, DWARF 4
  .debug_loc lists (neither backend writes them).
