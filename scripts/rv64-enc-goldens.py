#!/usr/bin/env python3
"""Regenerate the riscv64 encoder's golden table (decision 0645, stage 1).

    scripts/rv64-enc-goldens.py forms > forms.s       # the forms, one per marker label
    (linux host) llvm-mc -triple=riscv64 -mattr=+m,+a,+f,+d,+zicsr,+zifencei,+zba,+zbb \
        -riscv-no-aliases -show-encoding forms.s > forms.enc
    scripts/rv64-enc-goldens.py table forms.s forms.enc  # compiler/macho/enc_riscv64_test.xo

Each form follows a label `.Lf<N>:`, so llvm-mc's output (the expanded
instructions, each with `# encoding: [...]`) splits back into each form's
words. The test file keeps its code after the table (the line `]` that
ends it). The Mac's tools have no riscv64 target: run llvm-mc on a linux
host (LLVM 18 or later).
"""
import os, re, sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
TEST = os.path.join(ROOT, "compiler/macho/enc_riscv64_test.xo")

X = ["zero", "ra", "sp", "gp", "tp", "t0", "t1", "t2", "s0", "s1", "a0", "a1", "a2", "a3", "a4", "a5", "a6", "a7",
     "s2", "s3", "s4", "s5", "s6", "s7", "s8", "s9", "s10", "s11", "t3", "t4", "t5", "t6"]
XS = ["a0", "t6", "s11", "ra", "sp"]
F = ["ft0", "fa0", "fs11", "ft11", "f5"]


def forms():
    out = []
    regs3 = [("a0", "a1", "a2"), ("t6", "s11", "ra"), ("sp", "zero", "t0"), ("x5", "x31", "x17")]
    r = ["add", "sub", "sll", "slt", "sltu", "xor", "srl", "sra", "or", "and", "addw", "subw", "sllw", "srlw", "sraw",
         "mul", "mulh", "mulhsu", "mulhu", "div", "divu", "rem", "remu", "mulw", "divw", "divuw", "remw", "remuw",
         "sh1add", "sh2add", "sh3add", "add.uw", "sh1add.uw", "sh2add.uw", "sh3add.uw",
         "andn", "orn", "xnor", "max", "maxu", "min", "minu", "rol", "rolw", "ror", "rorw"]
    for m in r:
        for a, b, c in regs3:
            out.append(f"{m} {a}, {b}, {c}")
    for m in ["addi", "slti", "sltiu", "xori", "ori", "andi", "addiw"]:
        for imm in [0, 1, -1, 2047, -2048, 100]:
            out.append(f"{m} a0, a1, {imm}")
        out.append(f"{m} t6, s11, -7")
    for m, top in [("slli", 63), ("srli", 63), ("srai", 63), ("slliw", 31), ("srliw", 31), ("sraiw", 31), ("rori", 63), ("roriw", 31), ("slli.uw", 63)]:
        for sh in [0, 1, top]:
            out.append(f"{m} a0, a1, {sh}")
    for m in ["clz", "clzw", "ctz", "ctzw", "cpop", "cpopw", "sext.b", "sext.h", "zext.h", "rev8", "orc.b"]:
        out.append(f"{m} a0, a1")
        out.append(f"{m} t6, s11")
    for m in ["lb", "lh", "lw", "ld", "lbu", "lhu", "lwu"]:
        for off in [0, 8, -8, 2047, -2048]:
            out.append(f"{m} a0, {off}(sp)")
        out.append(f"{m} t6, 16(s11)")
    for m in ["sb", "sh", "sw", "sd"]:
        for off in [0, 8, -8, 2047, -2048]:
            out.append(f"{m} a0, {off}(sp)")
        out.append(f"{m} t6, 16(s11)")
    for m in ["flw", "fld"]:
        out.append(f"{m} fa0, 8(sp)")
        out.append(f"{m} fs11, -2048(a0)")
    for m in ["fsw", "fsd"]:
        out.append(f"{m} fa0, 8(sp)")
        out.append(f"{m} fs11, 2047(a0)")
    for v in [0, 1, -1, 2047, -2048, 2048, 4096, 0x12345, 0x7ffff7ff, -0x80000000, 0x12345678, -12345678]:
        out.append(f"li a0, {v}")
    out += ["lui a0, 0", "lui a0, 1", "lui t6, 1048575", "auipc a0, 0", "auipc t6, 1048575"]
    out += ["jal ra, 0", "jalr ra, 0(a0)", "jalr zero, 8(t0)", "jalr a1", "jr a0", "ret", "nop",
            "mv a0, a1", "not a0, a1", "neg a0, a1", "negw a0, a1", "sext.w a0, a1", "seqz a0, a1", "snez a0, a1",
            "sltz a0, a1", "sgtz a0, a1", "ecall", "ebreak", "fence", "fence rw, rw", "fence r, rw", "fence rw, w",
            "fence.tso", "fence.i"]
    for m in ["beq", "bne", "blt", "bge", "bltu", "bgeu", "bgt", "ble", "bgtu", "bleu"]:
        out.append(f"{m} a0, a1, 16")
        out.append(f"{m} t6, s11, -4096")
    for m in ["beqz", "bnez", "blez", "bgez", "bltz", "bgtz"]:
        out.append(f"{m} a0, 8")
    for w in ["w", "d"]:
        for o in ["", ".aq", ".rl", ".aqrl"]:
            out.append(f"lr.{w}{o} a0, (a1)")
            out.append(f"sc.{w}{o} a0, a2, (a1)")
            for a in ["amoswap", "amoadd", "amoxor", "amoand", "amoor", "amomin", "amomax", "amominu", "amomaxu"]:
                out.append(f"{a}.{w}{o} a0, a2, (a1)")
    for c in ["csrrw a0, fcsr, a1", "csrrs a0, fflags, zero", "csrrc a0, frm, a1", "csrrwi a0, fcsr, 1",
              "csrrsi a0, fflags, 31", "csrrci a0, frm, 2", "rdtime a0", "rdcycle a0", "rdinstret a0",
              "frcsr a0", "frrm a0", "frflags a0"]:
        out.append(c)
    for p in ["s", "d"]:
        for m in ["fadd", "fsub", "fmul", "fdiv"]:
            out.append(f"{m}.{p} fa0, fa1, fa2")
            out.append(f"{m}.{p} fs11, ft0, f5, rtz")
        out.append(f"fsqrt.{p} fa0, fa1")
        for m in ["fsgnj", "fsgnjn", "fsgnjx", "fmin", "fmax"]:
            out.append(f"{m}.{p} fa0, fa1, fa2")
        for m in ["feq", "flt", "fle"]:
            out.append(f"{m}.{p} a0, fa1, fa2")
        for m in ["fmadd", "fmsub", "fnmsub", "fnmadd"]:
            out.append(f"{m}.{p} fa0, fa1, fa2, fa3")
        for m in ["fmv", "fneg", "fabs"]:
            out.append(f"{m}.{p} fa0, fa1")
        out.append(f"fclass.{p} a0, fa1")
        for i in ["w", "wu", "l", "lu"]:
            out.append(f"fcvt.{i}.{p} a0, fa1, rtz")
            out.append(f"fcvt.{p}.{i} fa0, a1")
    out += ["fcvt.s.d fa0, fa1", "fcvt.d.s fa0, fa1", "fmv.x.d a0, fa1", "fmv.d.x fa0, a1", "fmv.x.w a0, fa1", "fmv.w.x fa0, a1"]
    return out


def table(src, enc):
    lines = forms_from(src)
    words = {}
    cur = None
    for l in open(enc):
        m = re.match(r"\s*\.Lf(\d+):", l)
        if m:
            cur = int(m.group(1))
            words[cur] = []
            continue
        m = re.search(r"encoding: \[([^\]]*)\]", l)
        if m and cur is not None:
            bs = [int(x, 16) for x in m.group(1).split(",")]
            if len(bs) != 4:
                sys.exit(f"form {cur}: a {len(bs)} byte encoding (the C extension is off)")
            words[cur].append(bs[0] | bs[1] << 8 | bs[2] << 16 | bs[3] << 24)
    rows = []
    for i, f in enumerate(lines):
        ws = words.get(i)
        if not ws:
            sys.exit(f"form {i} ({f}): no encoding in {enc}")
        rows.append('  ("%s", [%s]),' % (f, ", ".join("0x%08x" % w for w in ws)))
    head = ("// Generated by scripts/rv64-enc-goldens.py table: each form and the\n"
            "// words llvm-mc encodes for it (-triple=riscv64, no C); do not edit the\n// table.\n\n"
            "const RV64_FORMS: List[(Str, List[Int])] = [\n")
    tail = "]\n"
    if os.path.exists(TEST):
        t = open(TEST).read()
        k = t.find("\n]\n")
        if k >= 0:
            tail = t[k + 1:]
    open(TEST, "w").write(head + "\n".join(rows) + "\n" + tail)


def forms_from(src):
    out = []
    for l in open(src):
        l = l.strip()
        if l and not l.startswith(".") and not l.endswith(":"):
            out.append(l)
    return out


if __name__ == "__main__":
    if len(sys.argv) >= 2 and sys.argv[1] == "forms":
        print("\t.text")
        for i, f in enumerate(forms()):
            print(f".Lf{i}:\n\t{f}")
    elif len(sys.argv) == 4 and sys.argv[1] == "table":
        table(sys.argv[2], sys.argv[3])
    else:
        sys.exit(__doc__)
