# xo's native code by cause (decision 0443), 2026-10-30

Measured: `xo dev size --by-cause --top=10 ./compiler/xo` with the xo
built at main 0c9fa7557 (go run ./scripts/xobuild), darwin/arm64, on the Mac.
xo then builds compiler/xo with main's own code generation, after
2d51a8403 (fault exits zero a large struct result with one shared
zero_val call). The rows cover 63,305,156 of the binary's 63,305,612
`__text` bytes (99.9%). Last verified: 2026-10-30.

| cause | bytes | share | earlier today |
|---|---:|---:|---:|
| body | 19,372,784 | 30.6% | 26.2% |
| landing | 19,031,572 | 30.0% | 25.7% |
| rc | 14,269,272 | 22.5% | 19.9% |
| fault_exit | 6,322,096 | 9.9% | 22.0% |
| frame | 2,000,784 | 3.1% | 2.7% |
| fault_stub | 1,594,484 | 2.5% | 2.2% |
| inline_rt | 570,128 | 0.9% | 0.7% |
| data | 144,036 | 0.2% | 0.2% |
| total | 63,305,156 | | 71,615,504 |

"Earlier today" is the first run (decision 0443 Shipped), xo built
by an xo from before 2d51a8403: fault exits went from about 15.7 MB to
6.3 MB, and the total from 71.6 MB to 63.3 MB; the other causes kept
their bytes within a few percent, so their shares rose.

## Where each cause is

- landing (19.0 MB): build_native alone 4,157,848 bytes, then
  ssa.Builder.runner 528,140, lower 411,508, ssa.Builder.fe_replay
  331,548, check_program_ov 270,520. By module: ssa 5.84 MB, compiler/xo
  (main) 5.31 MB, arm64 1.89 MB, check 1.77 MB, gen 1.04 MB.
- rc (14.3 MB): build_native 814,688, arm64.G.emit 333,240,
  check.Checker.habit_call 196,740, arm64.G.x86_emit_once 196,164,
  arm64.G.emit_condbr 194,796. By module: check 3.26 MB, arm64 2.80 MB,
  main 1.58 MB, modules 1.45 MB, gen 1.05 MB.
- fault_exit (6.3 MB, 6,784 functions): still about 8.4 KB in each of
  many ssa.Builder methods (rc_prepare_incr 8,992, rc_prepare 8,608,
  specialize_pass 8,448, ambiguous_type_names, path_named_mods,
  match_tparams 8,440 each); ssa 2.42 MB, gen 1.08 MB, arm64 1.06 MB,
  check 0.97 MB. 2d51a8403 left these: the next lever for fault exits.
- frame (2.0 MB, 10,071 functions): lower 31,776, the parser's
  foreign_stmt 11,600.
- fault_stub (1.6 MB): arm64.G.inline_rt 10,628, llvm.Gen.inline_rt
  9,656.
- inline_rt (0.57 MB): runtime helpers inlined outside std/llrt;
  run_store 9,828, build_native 5,608.

The full report (top 10 per cause and module) is the tool's output; the
rows can be saved with --save and compared with --diff.
