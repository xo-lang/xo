# 0253: the architecture of `xo debug`

Status: Accepted (owner, 2026-10-17, through the lead: the design and the
per OS process calls below approved as written; written by Xo lang 3
around the design the owner chose)

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
   arm64 backend drops a reload of a spilled word that x16 or x17
   already holds (a pass over each function's final text, lsr.xo
   reload_reuse, Xo lang 4, 2026-10-17; gotcha
   arm64-scratch-reload-cache): the register mirrors the slot until a
   call, a write to it, an sp change, or a label a later point refers
   to, so a write to a spilled variable's stack slot can go unseen while
   the code uses the register copy. Reading is unaffected: the slot and
   the register agree. When `set` is built the pass describes the cache:
   per dropped reload, the pc range where x16 or x17 mirrors the slot,
   as a location list entry for the variable's word
   (DWARF 5 allows overlapping entries: both places are valid there),
   and the write updates every place the word lives at the stop's pc. A
   build without those entries gets no write to a spilled word, with a
   message naming the reason. Values in registers and the LLVM backend's
   locations (LLVM describes its own copies) are writable from the start
   of `set`.

## New OS capabilities (rule 7b: approved by the owner as written, 2026-10-17)

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
  constant 10). Not yet: compressed ELF sections, DWARF 4 .debug_loc
  lists (neither backend writes them).
- The xo_static global (2026-10-17): with debug information both
  backends describe rt.static_word's table as a global variable
  `xo_static` of type [256]Int (DW_TAG_array_type with a subrange). The
  arm64 backend writes it in the unit of the file that defines the table
  (DW_OP_addr of its label; in a split build, the last file, a unit
  with no functions when that file has none: low_pc and high_pc both
  the table's address). The LLVM backend lists a DIGlobalVariable in
  each module's unit's globals and attaches it to `@.xo.static` where a
  module defines that (LLVM writes DW_OP_addrx, which the reader resolves
  through the unit's .debug_addr). The text is unchanged (decision
  0252's gate). TestDwarfReader reads it from each build.
- Stage 2's parts that call no OS (2026-10-17, while the owner reviews
  the per OS calls): selfhost/debug lines.xo reads .debug_line (versions
  2 to 5) into rows with their statement flags, gives the file and line
  at an address, and the addresses of FILE:LINE; bpt.xo resolves a
  breakpoint (FILE:LINE: one address per function or inlined copy, its
  lowest, as lldb's locations; a function: its first statement past the
  prologue, the first row after its entry, and each inlined copy's
  start) and keeps the table (numbers, the traps to plant, the
  breakpoint at a trap's address, deletion); cmd.xo parses point 4's
  commands and short forms; value.xo reads a variable at a pc through a
  Target (registers and memory, which the per OS layer will give) and
  shows it by its DWARF type (integers, floats from their bits, booleans,
  structures, arrays, pointers; a gone or unreadable word shows as
  `<optimized out>` or `<unreadable>` without hiding the rest) and
  follows a field path. Tests: core_test.xo (commands, values from a
  fake thread, floats, breakpoints on hand made line tables) and
  TestDwarfReader (`xo dev dwarf --break=main.xo:11` is lldb's address
  for the arm64 and LLVM builds, `--pc` gives the line back).
- The unwinder (2026-10-17): selfhost/debug unwind.xo walks a stopped
  thread's frames from its registers and memory (Target) by the
  executable's unwind tables: ELF .eh_frame (DWARF CFI: CIEs, FDEs, the
  register rules, remember and restore state; DW_CFA_undefined on the
  return address ends the walk, the outermost frames' mark), Mach-O
  __unwind_info (regular and compressed pages; FRAME with its saved
  pairs, FRAMELESS, DWARF mode through __eh_frame), else the frame
  pointer chain; a caller keeps only the callee saved registers (x19 and
  up, fp, sp: a variable in x0 to x18 there is `<unavailable>`, as
  lldb shows it). For the stopped frame a compact FRAME encoding counts
  only once the frame record's store ran
  (gotchas/arm64-compact-unwind-leaf-fault-frames.md). Frames without
  DWARF are named from the symbol table (ELF .symtab, Mach-O LC_SYMTAB).
  snap.xo reads a stopped thread recorded as text (registers, memory) as
  a Target; `xo dev unwind --snap FILE [--dwarf FILE] [--locals] EXE`
  prints the frames, inlined calls as their own frames (each caller at
  its call's file and line, DW_AT_call_file through its unit's line
  table), and each frame's variables read in that frame. internal/arm64
  TestUnwind: lldb records the thread (testdata/unwind/snap.py) at a
  breakpoint in debugvars' sum and in a task's work after it parked on a
  channel and resumed on its own fiber stack, built by the arm64 backend
  (selfhost/macho and cc) and by LLVM; xo dev unwind's frames are lldb's,
  in order, with their lines or symbol offsets, and its values match
  (k, total, d; run_claimed's units). Linux (by hand, 2026-10-17): gdb
  16.3 in Docker recorded the fiber's stop (a gdb Python `snap` command
  like snap.py, reading /proc/PID/maps for the stack's end) on the arm64
  and LLVM builds; xo dev unwind on the Mac read the ELF's .eh_frame and
  gave gdb's frames in order, to xo_fiber_entry (its undefined return
  address), with gdb's values for run_claimed (t, units) and
  cabi.fiber_main (f); where gdb printed arguments in x0 to x18 of a
  caller (run_entry's), xo dev unwind says `<unavailable>`.
- The batch session (2026-10-17): selfhost/debug session.xo holds a
  program's DWARF, line, unwind, and symbol tables, the breakpoints, and
  a stopped thread's frames (vframes: inlined calls as frames, as lldb),
  and runs point 4's commands, each giving its text: break (the
  addresses and their lines), delete, backtrace (the selected frame
  marked), frame N, locals and print (read in the selected frame: a
  caller's at its call, with its own registers), threads, quit; run,
  continue, step, next, and finish answer that process control comes
  with stage 2, tasks and task with stage 4. Until then the thread is a
  snapshot: `xo dev debug --snap FILE [--dwarf FILE] -c CMD... EXE`
  echoes each command as `(xo) CMD` and prints its output (exit 1 when
  one failed). `xo debug --batch` becomes this with a live process.
  TestUnwind runs it on each build: frame N to sum or run_claimed, print
  k or units (lldb's values), break, and the refusal to run.
- --json (2026-10-17): the session's commands also give one JSON line
  each (session.xo exec_json), version 1 as spec 11.5 documents it (a
  top level `v`; fields added keep it, a removed or changed field takes
  the next, as the XOnnnn codes keep their meaning); `xo dev debug
  --json`. selfhost/debug json_test.xo holds a golden line per command
  on a hand made session; TestUnwind decodes the lines of a real one
  (v 1, ok, the printed value, the selected frame).
- The prompt (2026-10-17): with no -c, `xo dev debug` reads commands
  from standard input a line each until its end or quit, with the prompt
  `(xo) ` on a terminal and an empty line repeating the last command (as
  gdb and lldb); with --json that is the line protocol of spec 11.5 (one
  command in, one JSON line out). TestUnwind drives it through a pipe.
- Stage 2's process control on macOS arm64 (2026-11-05, Xo Lang 2):
  std/native/llrt/dbgmac.xo, reached through std/internal's `dbg_*`
  (Go backend stubs answer that a native xo is needed). It uses the
  approved calls and two more (below): posix_spawn with START_SUSPENDED and
  _POSIX_SPAWN_DISABLE_ASLR (lldb's flag; the executable then sits at its
  file's addresses, so the DWARF needs no slide), task_for_pid,
  mach_port_allocate and insert_right, task_set_exception_ports
  (BAD_ACCESS, BAD_INSTRUCTION, ARITHMETIC, SOFTWARE, BREAKPOINT;
  EXCEPTION_DEFAULT with 64 bit codes), mach_msg (a 50 ms receive loop
  with waitpid WNOHANG for the exit; the reply id + 100), mach_vm_read,
  mach_vm_protect (READ, WRITE and COPY: a private copy of the text page;
  macOS refuses a writable executable page) and mach_vm_write for a
  trap, thread_get_state and thread_set_state (ARM_THREAD_STATE64;
  ARM_DEBUG_STATE64's MDSCR_EL1.SS for one step over a trap; the kernel
  clears the bit before it reports, so the session tracks the step
  itself), task_resume, and task_threads with thread_suspend (not on the
  list) and thread_resume: a stop suspends every other thread of the program and
  continue resumes them (after the step over the trap, which runs the
  stopped thread alone), since the runtime's other threads otherwise
  fault executing on the page while a write has it without EXECUTE
  (EXC_BAD_ACCESS at their pc, seen in the first runs; kill SIGSTOP, on
  the list, would stop the stepping thread too). PT_ATTACHEXC was not
  needed. The other call beyond the list:
  mach_vm_machine_attribute(MATTR_CACHE, MATTR_VAL_CACHE_FLUSH) after each
  write, as debugserver does: without it a trap planted again after the
  step was not seen by the CPU in 19 of 20 runs (the program ran to its
  end), with it 20 of 20 stopped each time. Both wait for the owner's
  approval (lead, 2026-11-05) on the stage 2 branch.
  compiler/debug live.xo is the live session: `run` launches, plants and
  waits; `continue` steps over the trap and resumes; a stop's registers
  become the snapshot (its memory read from the process, snap.xo
  Snapshot.proc), so locals, print, backtrace and frame are the recorded
  thread's code. `xo debug [--batch] [--json] [-c CMD]... PROG [ARGS]`
  (compiler/cli/dev debugcmd.xo debug_main). Point 5: compiler/macho
  sign.xo writes entitlements into the ad hoc signature as codesign does
  (an empty requirements blob, the XML plist and DER blobs, their hashes
  in special slots 2, 5 and 7, not linker signed; byte for byte
  codesign's blobs for get-task-allow); a darwin build with debug
  information that is not --release or -s carries get-task-allow,
  `xo build --entitlement=KEY` adds others, internal/xotool gives the
  checkout's bin/xo com.apple.security.cs.debugger (when the seed knows
  the flag), and scripts/sign-macos.sh signs a release's xo with
  scripts/xo-entitlements.plist (the debugger's only; notarization
  refuses get-task-allow). Either entitlement is enough with Developer
  Mode on: an unentitled xo debugs a get-task-allow program, and the
  entitled xo a --release one; with neither, task_for_pid fails with 5
  and `run` says so. Tests: cmd/xo TestDebugBatch (a breakpoint in a
  loop, locals, a field, the backtrace through the inlined frame, four
  stops with the loop's values, the exit); 100 of 100 runs clean.
  Not yet: a multi-threaded program's threads listed (threads shows the
  stopped one), a thread made while stopped stays running, attach (-p).
- Stage 3's step, next and finish on the live session (2026-11-05, Xo
  Lang 2; compiler/debug step.xo): the stopped thread walked an
  instruction at a time (live.xo step1: the single step bit, the others
  suspended, a system call stepped with every thread running since it may
  wait for one; a trap at the pc taken out for it) to the start of a
  statement row of another line. A frame's depth is its canonical frame
  address (the unwinder's caller sp; the stack pointer alone failed: a
  breakpoint on a function's first line is before its prologue, so its
  whole body looked deeper) and then its inlined depth. next runs a bl or
  blr over with every thread to a temporary trap at the return address,
  matched by thread and stack pointer (recursion passes it at other
  depths), and judges lines at the starting frame's inlined depth: a call
  inlined from the next line counts as that line (in the arm64 backend's
  line table main.xo:17 and 18 had no rows of their own, only their
  inlined calls'), and a tail call (b into another function in the same
  frame) counts as a call. finish runs an inlined call until the pc
  leaves that call's ranges (the next code may be another inlined call at
  the same depth) and a function to its caller's return address in the
  caller's frame; after a stop in a deeper inlined call the frame the
  command was about is selected. step stops entering any frame, except a
  call into code without line information, which it runs over. Tests:
  cmd/xo TestDebugStep (next 15 to 16 to 17, step into the inlined sum,
  finish out of it with t = 76 in main, step into fact, next stopped by a
  breakpoint in the call it runs over, finish out of each recursive call
  with n and r); 10 of 10 repeated runs pass. Seen, not fixed: after
  fact returns, main's n prints the result (6): its location list keeps
  x0 past the call (a DWARF location matter, not the stepping).

