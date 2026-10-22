#!/bin/bash
# run.sh XO OUT: the runs behind the macOS debug info post (2026-10-22):
# a Go hello built with Go's defaults (compressed DWARF in __DWARF) and
# with -ldflags=-compressdwarf=false, and the same program in Xo (a
# .dSYM beside the binary), each read by Apple's atos, dwarfdump, lldb
# and dsymutil. XO is a darwin xo; OUT a scratch directory. Prints every
# command and its output; results.txt in this directory is one run.
set -u
xo=$1
out=$2
mkdir -p "$out/go" "$out/xo"
cd "$out/go"
cat > go.mod <<'G'
module hello
go 1.26
G
cat > main.go <<'G'
package main

//go:noinline
func greet(name string) string {
	return "hello, " + name
}

func main() {
	println(greet("world"))
}
G
cd "$out/xo"
cat > xo.mod <<'X'
module example.com/hello
edition preview
X
cat > main.xo <<'X'
// recursive, so the inliner keeps it a function of its own
fn greet(name: Str, n: Int) -> Str {
  if n <= 1 { "hello, ${name}" } else { greet(name, n - 1) }
}

fn main(os: Os) {
  os.stdio.println(greet("world", os.env.args().len()))
}
X
show() { echo "\$ $*"; "$@" 2>&1 | head -${LINES_MAX:-12}; echo; }
echo "== tools"
show go version
show sw_vers -productVersion
show xcrun --find atos
show "$xo" version
cd "$out/go"
go build -o hello-default .
go build -ldflags=-compressdwarf=false -o hello-plain .
for b in hello-default hello-plain; do
	echo "== go: $b"
	show ls -l "$b"
	echo "\$ otool -l $b | grep sectname | grep debug"
	otool -l "$b" | grep sectname | grep -E 'debug' | awk '{print $2}' | tr '\n' ' '; echo; echo
	addr=$(nm "$b" | awk '$3 == "_main.greet" {print $1}')
	echo "main.greet at 0x$addr"
	show atos -o "$b" -arch arm64 "0x$addr"
	LINES_MAX=6 show dwarfdump --debug-line "$b"
	show lldb -b -o "image lookup -v -n main.greet" "$b"
	show dsymutil "$b" -o "$b.dSYM"
	show ls "$b.dSYM/Contents/Resources/DWARF" 
done
cd "$out/xo"
XO_BIN_CACHE=off "$xo" build -o hello . 2>&1
echo "== xo: hello"
show ls -l hello
show ls hello.dSYM/Contents/Resources/DWARF
echo "\$ otool -l hello | grep sectname | grep debug"
otool -l hello | grep sectname | grep -E 'debug' | awk '{print $2}' | tr '\n' ' '; echo; echo
addr=$(nm hello | awk '$3 ~ /greet/ {print $1; exit}')
sym=$(nm hello | awk '$3 ~ /greet/ {print substr($3, 2); exit}')
echo "$sym at 0x$addr"
show atos -o hello -arch arm64 "0x$addr"
show atos -o hello.dSYM -arch arm64 "0x$addr"
LINES_MAX=6 show dwarfdump --debug-line hello.dSYM
show lldb -b -o "image lookup -v -n $sym" hello
show ls -l hello.dSYM/Contents/Resources/DWARF/hello
rm -rf hello.dSYM
echo "# hello.dSYM deleted: only what is in the executable is left"
show atos -o hello -arch arm64 "0x$addr"
LINES_MAX=14 show lldb -b -o "image lookup -v -n $sym" hello
echo "== other formats: Go cross builds for linux/arm64 and windows/amd64, Xo for linux/arm64"
cd "$out/go"
GOOS=linux GOARCH=arm64 go build -o hello-linux .
GOOS=windows GOARCH=amd64 go build -o hello-windows.exe .
echo "\$ objdump -h hello-linux | grep debug"
objdump -h hello-linux | grep -i debug; echo
echo "# an ELF SHF_COMPRESSED section starts with Elf64_Chdr: ch_type 1 is zlib, then the size, then 78 01 (zlib's header)"
LINES_MAX=5 show objdump -s -j .debug_abbrev hello-linux
echo "\$ objdump -h hello-windows.exe | grep debug"
objdump -h hello-windows.exe | grep -i debug; echo
cd "$out/xo"
XO_BIN_CACHE=off "$xo" build --targets=linux/arm64 -o hello-linux . 2>&1
echo "\$ objdump -h hello-linux | grep debug   # xo"
objdump -h hello-linux | grep -i debug; echo
LINES_MAX=5 show objdump -s -j .debug_abbrev hello-linux
echo "== Apple's own model: a C hello with clang -g, linked by ld"
mkdir -p "$out/c" && cd "$out/c"
printf '#include <stdio.h>\n\n__attribute__((noinline)) const char *greet(void) {\n  return "hello, world";\n}\n\nint main(void) {\n  puts(greet());\n  return 0;\n}\n' > hello.c
clang -g -O0 -c hello.c -o hello.o
clang hello.o -o hello
echo "\$ otool -l hello | grep sectname | grep debug   # nothing: ld leaves DWARF in hello.o"
otool -l hello | grep sectname | grep -c debug; echo
echo "\$ nm -ap hello | grep OSO   # the debug map: where the DWARF is"
nm -ap hello | grep OSO | sed "s|$out|\$OUT|"; echo
addr=$(nm hello | awk '$3 == "_greet" {print $1}')
show dsymutil hello
show ls -l hello.dSYM/Contents/Resources/DWARF/hello
rm hello.o
echo "# hello.o deleted: the DWARF is now only in hello.dSYM"
show atos -o hello -arch arm64 "0x$addr"
