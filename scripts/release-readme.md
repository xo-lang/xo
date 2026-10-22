# Xo @VERSION@ for @PLATFORM@

## Getting started

1. Put this directory's bin on your PATH. Keep bin and lib together:
   xo finds its standard library at ../lib/xo.

       export PATH="$PWD/xo-@VERSION@-@PLATFORM@/bin:$PATH"

   On Windows, in PowerShell:

       $env:Path = "$PWD\xo-@VERSION@-@PLATFORM@\bin;$env:Path"

2. Check it:

       xo version

3. A first program, in an empty directory:

       xo init example.com/hello

   then a main.xo of three lines:

       fn main(os: Os) {
         os.stdio.println("hello, world")
       }

   and run it:

       xo run .

4. Learn more from xo itself:

       xo doc Os          a type's fields and methods (also xo doc Map, xo doc Str)
       xo doc std/http    a std module
       xo explain XO0204  the long form of a diagnostic code

   The docs, the tour, and the language specification are at
   https://xolang.dev/docs/

---

