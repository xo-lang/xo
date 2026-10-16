// xo_rt.c: no C is left of the native runtime (decision 0234): the
// runtime is Xo (std/native/llrt), main is cabi_c.xo's and Windows' stack
// overflow handler entry.xo on_overflow. This empty file stays for the
// drivers of seeds before 0.16.0, which compile it into every native
// program (selfhost/xo link_inputs and rtpack); it goes once a seed
// whose driver does not.
