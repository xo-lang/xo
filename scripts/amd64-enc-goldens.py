#!/usr/bin/env python3
"""Regenerate the amd64 encoder's golden tables (decision 0254, stage 1).

    scripts/amd64-enc-goldens.py forms          # compiler/macho/enc_amd64_test.xo's table
    scripts/amd64-enc-goldens.py survey DIR     # enc_amd64_survey_test.xo's, from DIR/*.s
    scripts/amd64-enc-goldens.py objects        # asm_amd64_test.xo's, from compiler/macho/testdata/amd64/*.s
    scripts/amd64-enc-goldens.py macho-objects  # asm_macho_amd64_test.xo's, from compiler/macho/testdata/amd64-macho/*.s (macOS)

Each instruction is assembled by clang's integrated assembler (clang -c
--target=x86_64-unknown-linux-gnu, one section per instruction) and its
bytes read back with llvm-objdump: the encoder must give the same bytes.
`forms` is a fixed list of the forms the amd64 emitter writes; `survey`
takes the x86_64 assembly LLVM writes for the goldens (DIR/*.s, from a
linux/amd64 xo in Docker: xo asm --backend=llvm, clang -S -O2) and keeps
its symbol free lines, up to 30 per mnemonic. The test file keeps its
code after the table (the line `]` that ends it). `objects` assembles each
whole file with clang: the assembler must write the same ELF object.
"""
import collections, os, re, subprocess, sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
OBJDUMP = os.environ.get("LLVM_OBJDUMP", "/Library/Developer/CommandLineTools/usr/bin/llvm-objdump")


def forms():
    r64 = ["%rax", "%rcx", "%rdx", "%rbx", "%rsp", "%rbp", "%rsi", "%rdi", "%r8", "%r9", "%r10", "%r11", "%r12", "%r13", "%r14", "%r15"]
    r32 = ["%eax", "%ecx", "%edx", "%ebx", "%esp", "%ebp", "%esi", "%edi", "%r8d", "%r9d", "%r10d", "%r11d", "%r12d", "%r13d", "%r14d", "%r15d"]
    r16 = ["%ax", "%cx", "%dx", "%bx", "%sp", "%bp", "%si", "%di", "%r8w", "%r9w", "%r10w", "%r11w", "%r12w", "%r13w", "%r14w", "%r15w"]
    r8 = ["%al", "%cl", "%dl", "%bl", "%spl", "%bpl", "%sil", "%dil", "%r8b", "%r9b", "%r10b", "%r11b", "%r12b", "%r13b", "%r14b", "%r15b"]
    R = {"b": r8, "w": r16, "l": r32, "q": r64}
    mems = ["(%rax)", "8(%rax)", "-8(%rbp)", "(%rsp)", "16(%rsp)", "(%r12)", "(%r13)", "0(%rbp)", "300(%rbx)", "-300(%r15)", "(%rax,%rcx)", "(%rax,%rcx,8)", "4(%rsi,%rdi,4)", "(%r8,%r9,2)", "-1(%rsp,%rax,1)", "(,%rcx,8)", "16(,%rdx,4)", "(%rbp,%r13,8)"]
    ccs = ["o", "no", "b", "ae", "e", "ne", "be", "a", "s", "ns", "p", "np", "l", "ge", "le", "g"]
    L = []
    for op in ["add", "or", "adc", "sbb", "and", "sub", "xor", "cmp"]:
        for sfx in "bwlq":
            r = R[sfx]
            L += [f"{op}{sfx} {r[1]}, {r[3]}", f"{op}{sfx} {r[10]}, {r[2]}", f"{op}{sfx} {r[0]}, {r[13]}", f"{op}{sfx} {r[5]}, 8(%rax)", f"{op}{sfx} -8(%rbp), {r[6]}", f"{op}{sfx} (%r12,%rcx,4), {r[9]}"]
            for imm in ["1", "-1", "127", "-128", "128", "300", "-129"]:
                if sfx == "b" and imm in ("300", "-129", "128"):
                    continue
                L += [f"{op}{sfx} ${imm}, {r[0]}", f"{op}{sfx} ${imm}, {r[3]}", f"{op}{sfx} ${imm}, {r[11]}", f"{op}{sfx} ${imm}, 16(%rsp)"]
    for sfx in "bwlq":
        r = R[sfx]
        for a, b in [(1, 3), (0, 8), (12, 4), (6, 15)]:
            L.append(f"mov{sfx} {r[a]}, {r[b]}")
        for m in mems:
            L += [f"mov{sfx} {r[2]}, {m}", f"mov{sfx} {m}, {r[9]}"]
        for imm in ["0", "1", "-1", "127", "300", "-300", "2147483647", "-2147483648"]:
            if sfx == "b" and imm not in ("0", "1", "-1", "127"):
                continue
            if sfx == "w" and imm in ("2147483647", "-2147483648"):
                continue
            L += [f"mov{sfx} ${imm}, {r[0]}", f"mov{sfx} ${imm}, {r[11]}", f"mov{sfx} ${imm}, 8(%rbp)"]
    L += ["movabsq $81985529216486895, %rax", "movabsq $-81985529216486895, %r11", "movq $4294967295, %rcx"]
    for m in mems:
        L += [f"leaq {m}, %rax", f"leaq {m}, %r14"]
    L += ["leal 4(%rax,%rcx), %edx"]
    for sfx in "bwlq":
        r = R[sfx]
        L += [f"test{sfx} {r[1]}, {r[3]}", f"test{sfx} {r[0]}, {r[0]}", f"test{sfx} {r[9]}, 8(%rax)", f"test{sfx} $1, {r[0]}", f"test{sfx} $1, {r[3]}", f"test{sfx} $1, {r[12]}"]
        for op in ["inc", "dec", "not", "neg", "mul", "imul", "div", "idiv"]:
            L += [f"{op}{sfx} {r[3]}", f"{op}{sfx} {r[13]}", f"{op}{sfx} 8(%rax)"]
        for op in ["shl", "shr", "sar", "rol", "ror"]:
            L += [f"{op}{sfx} $1, {r[3]}", f"{op}{sfx} $5, {r[3]}", f"{op}{sfx} $5, {r[10]}", f"{op}{sfx} %cl, {r[3]}", f"{op}{sfx} %cl, 8(%rax)", f"{op}{sfx} {r[3]}"]
    for sfx in "wlq":
        r = R[sfx]
        L += [f"imul{sfx} {r[1]}, {r[3]}", f"imul{sfx} 8(%rax), {r[10]}", f"imul{sfx} $3, {r[1]}, {r[3]}", f"imul{sfx} $300, {r[1]}, {r[3]}", f"imul{sfx} $3, 8(%rax), {r[3]}"]
        for op in ["bsf", "bsr", "tzcnt", "lzcnt", "popcnt"]:
            L += [f"{op}{sfx} {r[1]}, {r[3]}", f"{op}{sfx} 8(%rax), {r[10]}"]
        for cc in ccs:
            L.append(f"cmov{cc}{sfx} {r[1]}, {r[3]}")
        L += [f"cmove{sfx} 8(%rax), {r[11]}"]
    for cc in ccs:
        L += [f"set{cc} %al", f"set{cc} %r9b", f"set{cc} %sil"]
    L += ["bswapl %eax", "bswapl %r9d", "bswapq %rax", "bswapq %r12"]
    for a, b in [("movzbl", "%al, %ecx"), ("movzbl", "%r9b, %r10d"), ("movzbl", "8(%rax), %ecx"), ("movzbl", "%sil, %eax"), ("movzwl", "%ax, %ecx"), ("movzwl", "8(%rax), %r11d"), ("movsbl", "%al, %ecx"), ("movswl", "%cx, %edx"), ("movsbq", "%al, %rcx"), ("movswq", "%r9w, %rax"), ("movslq", "%eax, %rcx"), ("movslq", "8(%rax), %r12"), ("movzbq", "%al, %rcx"), ("movzwq", "%ax, %r15"), ("movsbw", "%al, %cx"), ("movzbw", "%r8b, %ax")]:
        L.append(f"{a} {b}")
    for r in r64:
        L += [f"pushq {r}", f"popq {r}"]
    L += ["pushq $1", "pushq $300", "pushq 8(%rax)", "popq 8(%rax)", "retq", "ret", "cqto", "cltd", "cltq", "cwtl", "ud2", "int3", "nop", "hlt", "pause", "lfence", "mfence", "sfence"]
    L += ["callq *%rax", "callq *%r11", "callq *8(%rax)", "jmpq *%rax", "jmpq *%r9", "jmpq *(%rax,%rcx,8)", "call *%rax", "jmp *%rcx"]
    for sfx in "bwlq":
        r = R[sfx]
        L += [f"xchg{sfx} {r[1]}, {r[3]}", f"xchg{sfx} {r[1]}, 8(%rax)", f"lock cmpxchg{sfx} {r[1]}, 8(%rax)", f"cmpxchg{sfx} {r[9]}, (%rdi)", f"lock xadd{sfx} {r[1]}, 8(%rax)", f"xadd{sfx} {r[1]}, {r[3]}"]
    L += ["lock addq $1, (%rax)", "lock incq (%rax)", "lock decl 8(%rbx)", "lock orq %rax, (%rdx)"]
    x = [f"%xmm{i}" for i in range(16)]
    for op in ["movsd", "movss"]:
        L += [f"{op} {x[1]}, {x[2]}", f"{op} {x[9]}, {x[3]}", f"{op} 8(%rax), {x[1]}", f"{op} {x[12]}, -8(%rbp)", f"{op} (%r12,%rcx,8), {x[15]}"]
    for op in ["addsd", "subsd", "mulsd", "divsd", "sqrtsd", "minsd", "maxsd", "addss", "subss", "mulss", "divss", "sqrtss", "ucomisd", "comisd", "ucomiss", "cvtsd2ss", "cvtss2sd", "xorps", "xorpd", "andpd", "andnpd", "orpd", "andps", "movaps", "movapd", "movups", "movupd", "unpcklpd"]:
        L += [f"{op} {x[1]}, {x[2]}", f"{op} {x[10]}, {x[3]}", f"{op} 8(%rax), {x[4]}"]
    L += ["cvtsi2sdq %rax, %xmm0", "cvtsi2sdq %r12, %xmm9", "cvtsi2sdl %eax, %xmm1", "cvtsi2sdq 8(%rax), %xmm2", "cvttsd2si %xmm0, %rax", "cvttsd2si %xmm9, %r12", "cvttsd2si %xmm0, %eax", "cvtsi2ssq %rax, %xmm0", "cvttss2si %xmm1, %rcx", "cvtsd2si %xmm0, %rax"]
    L += ["movq %rax, %xmm0", "movq %xmm0, %rax", "movq %r12, %xmm9", "movq %xmm15, %r8", "movd %eax, %xmm1", "movd %xmm1, %ecx", "movq %xmm1, %xmm2", "movq 8(%rax), %xmm3", "movq %xmm3, 8(%rax)"]
    # the crypto set of stage 3
    for op in ["aesenc", "aesenclast", "aesdec", "aesdeclast", "aesimc", "sha256msg1", "sha256msg2", "pshufb", "pxor", "movdqa", "movdqu"]:
        L += [f"{op} {x[1]}, {x[2]}", f"{op} {x[9]}, {x[3]}", f"{op} 16(%rax), {x[12]}"]
    L += ["movdqu %xmm1, (%rdi)", "movdqu %xmm9, 16(%r8)", "movdqa %xmm2, -32(%rbp)"]
    L += ["sha256rnds2 %xmm0, %xmm1, %xmm2", "sha256rnds2 %xmm0, 16(%rsi), %xmm9"]
    L += ["aeskeygenassist $1, %xmm1, %xmm2", "aeskeygenassist $54, %xmm9, %xmm3", "pclmulqdq $0, %xmm1, %xmm2", "pclmulqdq $17, %xmm9, %xmm3", "pclmulqdq $16, 16(%rax), %xmm4"]
    L += ["pshufd $27, %xmm1, %xmm2", "pshufd $78, %xmm9, %xmm10", "pslldq $4, %xmm1", "psrldq $8, %xmm9", "psrlq $63, %xmm2", "psllq $1, %xmm3"]
    L += ["adcxq %rax, %rbx", "adcxq %r9, %r10", "adcxq 8(%rsi), %rcx", "adoxq %rax, %rbx", "adoxq %r11, %r12", "adcxl %eax, %ebx"]
    L += ["mulxq %rax, %rbx, %rcx", "mulxq %r9, %r10, %r11", "mulxq 8(%rsi), %r8, %r9", "mulxq %rdi, %rax, %rdx"]
    L += ["pinsrq $1, %rax, %xmm0", "pinsrq $0, %r9, %xmm10", "pextrq $1, %xmm0, %rax", "pextrq $0, %xmm11, %r12", "pinsrd $2, %eax, %xmm1", "pextrd $3, %xmm1, %ecx"]
    L += ["btq $62, %rax", "btq %rcx, %rdx", "shldq $5, %rax, %rbx", "shrdq %cl, %rax, %rbx", "rep bsfq %rax, %rbx", "syscall", "cpuid", "stc", "clc", "roundsd $9, %xmm1, %xmm2", "roundsd $10, %xmm9, %xmm3", "roundss $11, 8(%rax), %xmm4", "shrdq $13, %rax, %rbx", "rorl $7, %ecx", "popcntq %r9, %r10"]
    return L


def survey(d):
    reg = r"%[a-z0-9]+"
    num = r"-?(?:0x[0-9a-f]+|\d+)"
    mem = rf"(?:%[fg]s:)?(?:{num})?\((?:{reg})?(?:,{reg}(?:,\d)?)?\)"
    opre = re.compile(rf"^\*?(?:{reg}|\${num}|{mem}|%[fg]s:{num})$")
    seen = collections.defaultdict(list)
    have = set()
    for f in sorted(x for x in os.listdir(d) if x.endswith(".s")):
        for l in open(os.path.join(d, f)):
            s = l.split("#")[0].strip()
            if not s or s.endswith(":") or s.startswith(".") or s.startswith('"'):
                continue
            s = re.sub(r"\s+", " ", s)
            parts = s.split(" ", 1)
            m = parts[0]
            if m in ("lock", "rep"):
                sub = parts[1].split(" ", 1)
                m = m + " " + sub[0]
                rest = sub[1] if len(sub) > 1 else ""
            else:
                rest = parts[1] if len(parts) > 1 else ""
            ops = [o.strip() for o in re.split(r",(?![^()]*\))", rest)] if rest else []
            if any(not opre.match(o) for o in ops):
                continue
            line = (m + " " + ", ".join(ops)).strip()
            if line in have or len(seen[m]) >= 30:
                continue
            have.add(line)
            seen[m].append(line)
    return [l for m in sorted(seen) for l in seen[m]]


def encodings(lines, tmp):
    s = os.path.join(tmp, "enc.s")
    o = os.path.join(tmp, "enc.o")
    open(s, "w").write("".join('.section .t%d,"ax",@progbits\n%s\n' % (i, l) for i, l in enumerate(lines)))
    subprocess.run(["clang", "-c", "--target=x86_64-unknown-linux-gnu", s, "-o", o], check=True)
    dis = subprocess.run([OBJDUMP, "-d", "--no-addresses", o], capture_output=True, text=True, check=True).stdout
    by = {}
    cur = None
    for l in dis.split("\n"):
        m = re.match(r"Disassembly of section \.t(\d+):", l)
        if m:
            cur = int(m.group(1))
            by[cur] = ""
            continue
        if cur is not None and l.startswith(" ") and "\t" in l:
            by[cur] += l.split("\t")[0].strip().replace(" ", "")
    return [by[i] for i in range(len(lines))]


def write(path, const, header, lines, hexes):
    old = open(path).read() if os.path.exists(path) else ""
    tail = old[old.index("\n]\n") + 3:] if "\n]\n" in old else "\n"
    out = header + ["", "const %s: List[(Str, Str)] = [" % const]
    out += ['  ("%s", "%s"),' % (l, h) for l, h in zip(lines, hexes)]
    out.append("]")
    open(path, "w").write("\n".join(out) + "\n" + tail)


def main():
    import tempfile
    tmp = tempfile.mkdtemp()
    if sys.argv[1] == "forms":
        L = forms()
        write(os.path.join(ROOT, "compiler/macho/enc_amd64_test.xo"), "AMD64_ENCODINGS",
              ["// Generated by scripts/amd64-enc-goldens.py forms: each form as clang's", "// x86_64 assembler encodes it (clang -c --target=x86_64-unknown-linux-gnu,", "// llvm-objdump -d); do not edit the table."], L, encodings(L, tmp))
    elif sys.argv[1] == "survey":
        L = survey(sys.argv[2])
        write(os.path.join(ROOT, "compiler/macho/enc_amd64_survey_test.xo"), "AMD64_SURVEY",
              ["// Generated by scripts/amd64-enc-goldens.py survey: the symbol free lines", "// (up to 30 per mnemonic) of the x86_64 assembly LLVM writes for the", "// goldens, as clang's assembler encodes them; do not edit the table."], L, encodings(L, tmp))
    elif sys.argv[1] == "objects":
        d = os.path.join(ROOT, "compiler/macho/testdata/amd64")
        L = []
        hexes = []
        for f in sorted(x for x in os.listdir(d) if x.endswith(".s")):
            src = os.path.join(d, f)
            o = os.path.join(tmp, f + ".o")
            subprocess.run(["clang", "-c", "--target=x86_64-unknown-linux-gnu", src, "-o", o], check=True)
            L.append((f, open(src).read(), open(o, "rb").read().hex()))
        path = os.path.join(ROOT, "compiler/macho/asm_amd64_test.xo")
        old = open(path).read() if os.path.exists(path) else ""
        tail = old[old.index("\n]\n") + 3:] if "\n]\n" in old else "\n"
        out = ["// Generated by scripts/amd64-enc-goldens.py objects: each", "// compiler/macho/testdata/amd64/*.s and the ELF object clang writes for it", "// (clang -c --target=x86_64-unknown-linux-gnu); do not edit the table.", "", "const AMD64_OBJECTS: List[(Str, Str, Str)] = ["]
        for f, src, h in L:
            out.append('  ("%s", r#"%s"#, "%s"),' % (f, src, h))
        out.append("]")
        open(path, "w").write("\n".join(out) + "\n" + tail)
    elif sys.argv[1] == "macho-objects":
        # asm_macho_amd64_test.xo's (decision 0254 stage 6a): Mach-O x86_64
        # objects as `clang -c -arch x86_64 -mmacosx-version-min=11.0` writes them
        d = os.path.join(ROOT, "compiler/macho/testdata/amd64-macho")
        L = []
        for f in sorted(x for x in os.listdir(d) if x.endswith(".s")):
            src = os.path.join(d, f)
            o = os.path.join(tmp, f + ".o")
            subprocess.run(["clang", "-c", "-arch", "x86_64", "-mmacosx-version-min=11.0", src, "-o", o], check=True)
            L.append((f, open(src).read(), open(o, "rb").read().hex()))
        path = os.path.join(ROOT, "compiler/macho/asm_macho_amd64_test.xo")
        old = open(path).read() if os.path.exists(path) else ""
        tail = old[old.index("\n]\n") + 3:] if "\n]\n" in old else "\n"
        out = ["// Generated by scripts/amd64-enc-goldens.py macho-objects: each", "// compiler/macho/testdata/amd64-macho/*.s and the Mach-O object clang writes", "// for it (clang -c -arch x86_64 -mmacosx-version-min=11.0); do not edit", "// the table.", "", "const AMD64_MACHO_OBJECTS: List[(Str, Str, Str)] = ["]
        for f, src, h in L:
            out.append('  ("%s", r#"%s"#, "%s"),' % (f, src, h))
        out.append("]")
        open(path, "w").write("\n".join(out) + "\n" + tail)
    print(len(L), "lines")


main()
