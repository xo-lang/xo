# xotool.CompileC linked no TLS library after the Go compiler went

Status: Observed, fixed (2026-10-13, netfix agent; Mac and linux/arm64). Obsolete since 2026-10-15: Mbed TLS removed (decision 0202).

Symptom: after decision 0215, internal/llvm's TestGoldenTasks (the
ThreadSanitizer builds of the goldens that use tasks) failed to link
http_wire, net_only, http_server, http_fault, and http_client with
`undefined reference to 'xo_tls_roots_system'` and `'xo_srv_tls'`
(`Undefined symbols for architecture arm64` on the Mac). TestGoldenASan
failed the same way. Programs built by `xo build` were fine.

Cause: those tests take the program's LLVM IR from xo and compile and
link it with the C runtime themselves (xotool.CompileC, so they can add
`-fsanitize=`). The Go build package's CompileC that they used before
0215 noticed IR calling std/http's runtime (xo_cl_do, xo_srv_new,
xo_tls_*) and added -DXO_TLS, the Mbed TLS include directories, and
libxotls.a; its replacement in internal/xotool did not. Without
-DXO_TLS, xo_tls.c defines none of the functions the IR calls.

Fix: internal/xotool/tls.go is internal/build's tls.go again, and
CompileC adds its compile flags and the library (built for the
sanitizer flavor of the flags, cached in <user cache>/xo/mbedtls) when
an .ll source needs TLS. compiler/xo/tls.xo does the same for xo.

Avoid: a test helper that replaces a build path must keep every input
the old one added; a link that "only" lost a library fails only for the
programs that need it, here 5 of the 20 task goldens.
