# std/native/llrt functions no golden reaches (2026-10-18)

The lead's request after the dead function survey: the runtime
functions that no golden program reaches but that do have a call site,
to cover with tests (decision 0262's std module tests are the natural
place for glob, asn1, tzhost and the cpu fallbacks).

## Method

- Reachability in the LLVM IR of every golden (internal/llvm/testdata/
  golden, 194 programs) on darwin/arm64, with and without --race, from
  the module's external definitions and main. The IR is lowered before
  any inlining, so a function inlined into all its callers still counts
  as reached (the arm64 backend's dead function removal, which runs after
  inlining, cannot answer this: it drops inlined-away callees).
- Union with the functions the native linux/arm64 and linux/amd64
  builds of the same programs emit (XO_DCE=list survey, `xo asm`).
- Left out: code only `xo test` runs (testworld, twrun, twflags, gen),
  XO_NOLIBC builds (libc*, cabi_libc, cabi_nolibc), and Windows code
  (windows.xo, win* functions), none of which the goldens build.
- Not covered: LLVM builds for Linux and Windows (the LLVM backend builds
  for the host only without a sysroot) and wasm (its own runtime in
  compiler/wasm; it reaches llrt only through `target.os` arms).

Of 3,569 llrt functions, 573 were unreached on darwin LLVM, 461 after the
Linux union. 22 had no call site, string reference or export role: 19
were deleted (the same change as this note; owners confirmed), and 3 are
kept: cabi_tw.tw_kick (the test world calls it through its address),
ct.mask_lt (decision 0222's constant-time helper set), linux.sig_restorer
(nolibc signal setup; owner asked). The rest, below, have call sites on
paths no golden takes.

## The list (141 functions)

| File | Count | Functions |
|---|---|---|
| cpu | 15 | lowered, cpuid1_ecx, cpuid7_ebx, carry, borrow, adc, adc_carry, clmul_lo, clmul_hi, aes_round, aes_last, aes_enc10, aes_enc12, aes_enc14, sha256_block |
| dns | 15 | gai_text, is_space, v4_at, v4_fields, space_at, name_is, is_domain_name, u16, server_addr, only, zeros, loopback6, zero_to, common_prefix, before |
| tzhost | 11 | i32, abbrev, caps, ascii, pseudo_unix, days_in, days_from_civil, year_of, tzif, be, header |
| prof | 10 | darwin, supported, unsupported_note, loc_index, coff_symbols, u16, u32, zstr, zstr_n, read_self |
| linux | 9 | read, is_dir, clock_gettime, elf_dynsym, mprotect, sig_restorer, fs_status, nanosleep_at, auxv_init |
| sock | 8 | loopback_pair, wret, recv, ioctl, epoll_wait, ppoll, sockaddr_ip, connect_to |
| asn1 | 7 | raw, bits, text, digits, days, month_days, time |
| glob | 7 | rune_at, get_esc, match_class, chunk_failed, match_chunk, chunk_end, matches |
| proc | 5 | lock, unlock, record, tw_reset, killed |
| stdin | 5 | linux_fstat_mode, set_tty, tw_reset, tty_width, i16 |
| obj | 4 | resize_obj, rs_new_obj, rt_queued, rt_heap |
| unwind | 4 | lookup, after_call, call_ff, pdata_find |
| logstate | 3 | configured, reset, set_from |
| poll | 3 | afd_done, afd_open, env_is |
| pool | 3 | scope_listed, alive, cpus |
| report | 3 | prop_report, ult, shorter |
| stop | 3 | reset, reset_test, unroot |
| fsops | 2 | no_file, no_lock |
| http1 | 2 | go_quote_c, url_error |
| raw | 2 | fill, set_io_chunk |
| tlscert | 2 | cert_files, days_from_civil |
| alloc | 1 | refill |
| atexit | 1 | set_tw_end |
| bigmod | 1 | limbs |
| cabi_tw | 1 | tw_end |
| ct | 1 | mask_lt |
| darwin | 1 | clock_ns |
| mutex | 1 | queued |
| netconn | 1 | os |
| poolsetup | 1 | online_count |
| process | 1 | next_wstr |
| rtsec | 1 | allocs |
| sysio | 1 | clock_buf |
| tls12 | 1 | ready |
| tls12srv | 1 | ready |
| tls13 | 1 | recv_some |
| tracerec | 1 | built |
| tz | 1 | local_name |
| tzrules | 1 | parse_unkept |
