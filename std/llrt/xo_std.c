// xo_std.c: the C of the native std implementation (decision 0074),
// included at the end of xo_rt.c. Its intrinsics are Xo now
// (std/native/llrt); what is left is the files below.

// the fault of an index out of range on Bytes (decisions 0079, 0081)
#include "xo_bytes.c"

// HTTP/1.1 (std.md 4)
#include "xo_http.c"

// std/time zones (decision 0094): only in programs that use std/time,
// which the build compiles with XO_TIME and their zone data (xo_tzdata.c).
// The zone data and the kept zones are Xo (std/native/llrt/tzhost.xo,
// work item B27); the leak check frees the kept zones through
// cabi_time.xo's export (xo_rt.c's end of main, under XO_TIME: no static
// constructor, which selfhost/macho's ELF link does not run).
